package loader

import (
	"fmt"
	"sync"
	"time"

	"globaldevtools.bbva.com/entsec/semaas/omega/api"
)

// OmegaClient defines the methods that the omega client needs to have
type OmegaClient interface {
	Load(entries []*api.LogEntry) error
}

// Loader defines a type of logger that is able to send messages to omega
type Loader interface {
	Load(entry *api.LogEntry) error
	Close() error
}

type bulkLoader struct {
	opts bulkLoadOptions

	failed int

	sendCh chan struct{}
	stopCh chan struct{}
	closed bool
	buffer *LogEntryBuffer
	client OmegaClient

	lock     sync.Mutex
	loadLock sync.Mutex
}

type bulkLoadOptions struct {
	timeout time.Duration

	bulkSize int
	bulkMsgs int

	maxRetries    int
	retryWaitTime time.Duration

	fallbackSize   int
	fallbackStorer FallbackStorer

	maxSizeThreshold int
}

type BulkLoadOption func(*bulkLoadOptions)

func WithTimeout(timeout time.Duration) BulkLoadOption {
	return func(o *bulkLoadOptions) {
		o.timeout = timeout
	}
}

func WithBulkSize(size int, msgs int) BulkLoadOption {
	return func(o *bulkLoadOptions) {
		o.bulkSize = size * 1024
		o.bulkMsgs = msgs
	}
}

func WithRetries(retries int, wait time.Duration) BulkLoadOption {
	return func(o *bulkLoadOptions) {
		o.maxRetries = retries
		o.retryWaitTime = wait
	}
}

func WithFallback(size int, storer FallbackStorer) BulkLoadOption {
	return func(o *bulkLoadOptions) {
		o.fallbackSize = size * 1024
		o.fallbackStorer = storer
	}
}

// WithSafeThreshold adds a threshold value to empty memory when,
// due to several failures, logs can't be send. The size is
// specifed in KB
func WithSafeThreshold(size int) BulkLoadOption {
	return func(o *bulkLoadOptions) {
		o.maxSizeThreshold = size * 1024
	}
}

// Bulk returns a Loader that sends messages in bulk to omega
func Bulk(c OmegaClient, opts ...BulkLoadOption) (Loader, error) {
	if c == nil {
		return nil, fmt.Errorf("Client can't be nil. A client must be provided")
	}
	l := &bulkLoader{
		sendCh: make(chan struct{}),
		stopCh: make(chan struct{}),
		buffer: NewLogEntryBuffer(40),
		client: c,
	}
	l.setDefaultOpts()
	for _, opt := range opts {
		opt(&l.opts)
	}

	go l.load()
	return l, nil
}

// Load adds an entry to be send to omega by the bulk loader
func (l *bulkLoader) Load(entry *api.LogEntry) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.closed {
		return fmt.Errorf("Error: Can't load messages into a closed Loader")
	}
	if entry != nil {
		if err := l.buffer.Add(entry); err != nil {
			return err
		}
		if l.reachedBulkSize() {
			l.send(0)
		}
	}
	return nil
}

func (l *bulkLoader) setDefaultOpts() {
	opts := []BulkLoadOption{
		WithTimeout(10 * time.Second),
		WithBulkSize(512, 30),
		WithSafeThreshold(1024),
	}
	for _, opt := range opts {
		opt(&l.opts)
	}
}

func (l *bulkLoader) load() error {
	var err error
	for {
		select {
		case <-l.stopCh:
			return err // we return the last error value
		case <-l.sendCh:
		case <-time.After(l.opts.timeout):
		}
		err = l.bulkLoad(false)
	}
}

func (l *bulkLoader) bulkLoad(force bool) (err error) {
	l.loadLock.Lock()
	defer l.loadLock.Unlock()
	if l.buffer.Len() == 0 {
		return nil
	}

	// we double check the len of the buffer, we could have reached this state because len() was called during an upload
	if !l.reachedBulkSize() && !force {
		return nil
	}

	entries := l.buffer.Entries()
	if err = l.client.Load(entries); err != nil {
		size := l.buffer.Size()
		// Increase some counter of failures and if a lot fail then we save the logs on disk and try to process them later
		if l.failed++; l.failed <= l.opts.maxRetries {
			go l.send(l.opts.retryWaitTime)
			return nil
		}
		if l.opts.fallbackStorer != nil && size > l.opts.fallbackSize {
			var s LogEntryStorer
			if s, err = l.opts.fallbackStorer.Open("test"); err == nil {
				err = s.Save(entries)
			}
		}
		if err != nil {
			if size > l.opts.maxSizeThreshold { // Error sending messages and safe threshold reached. We clean anyway
				err = fmt.Errorf("Error sending messages")
			}
		}
	}
	l.failed = 0
	l.buffer.Clean()
	return err
}

func (l *bulkLoader) reachedBulkSize() bool {
	return l.buffer.Size() > l.opts.bulkSize || l.buffer.Len() >= l.opts.bulkMsgs
}

func (l *bulkLoader) send(after time.Duration) {
	<-time.After(after)
	l.sendCh <- struct{}{}
}

// Close sends the remaining entries to omega and sets the
// bulkloader as closed
func (l *bulkLoader) Close() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.closed {
		return nil
	}
	close(l.stopCh)
	l.closed = true
	return l.bulkLoad(true)
}
