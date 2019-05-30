package loader

import (
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"globaldevtools.bbva.com/entsec/semaas/omega/api"
)

// OmegaClient defines the methods that the omega client needs to have
type OmegaClient interface {
	Load(entries []*api.LogEntry) error
}

// Loader defines a type of logger that is able to send messages to omega
type Loader struct {
	opts bulkLoadOptions

	failed int

	sendCh  chan struct{}
	stopCh  chan struct{}
	closed  bool
	sending bool
	buffer  *LogEntryBuffer
	client  OmegaClient

	lock     sync.RWMutex
	sendLock sync.RWMutex
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

	sysLogger *log.Logger
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

func WithSystemLogger(l *log.Logger) BulkLoadOption {
	return func(o *bulkLoadOptions) {
		o.sysLogger = l
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
func Bulk(c OmegaClient, opts ...BulkLoadOption) (*Loader, error) {
	if c == nil {
		return nil, fmt.Errorf("Client can't be nil. A client must be provided")
	}
	l := &Loader{
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
func (l *Loader) Load(entry *api.LogEntry) error {
	if entry.Message == "" {
		return nil
	}
	if l.isClosed() {
		return fmt.Errorf("Error: Can't load messages into a closed Loader")
	}
	if entry != nil {
		if err := l.buffer.Add(entry); err != nil {
			return err
		}
		if l.reachedBulkSize() {
			// We set the need to send after the buffer is full. This will queue a request for sending
			// if there're no request at the moment
			go l.send(0)
		}
	}
	return nil
}

func (l *Loader) setDefaultOpts() {
	opts := []BulkLoadOption{
		WithTimeout(10 * time.Second),
		WithBulkSize(512, 30),
		WithSafeThreshold(1024),
		WithSystemLogger(log.New(ioutil.Discard, "", log.LstdFlags)),
	}
	for _, opt := range opts {
		opt(&l.opts)
	}
}

func (l *Loader) load() error {
	var err error
	for {
		select {
		case <-l.stopCh:
			return err // we return the last error value
		case <-l.sendCh:
		case <-time.After(l.opts.timeout):
		}
		err = l.bulkLoad()
	}
}

func (l *Loader) bulkLoad() (err error) {
	l.loadLock.Lock()
	defer l.loadLock.Unlock()
	// We could call this becaouse a previous Len() call could give us something but it got clean later
	if l.buffer.Len() == 0 {
		return nil
	}

	entries := l.buffer.Entries()
	if err = l.client.Load(entries); err != nil {
		l.opts.sysLogger.Printf("[ERR] Unable to load entries: %s", err)
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
	l.opts.sysLogger.Printf("[DEBUG] Sent %d entries to omega", len(entries))
	l.failed = 0
	l.buffer.Clean()
	return err
}

func (l *Loader) reachedBulkSize() bool {
	return l.buffer.Size() > l.opts.bulkSize || l.buffer.Len() >= l.opts.bulkMsgs
}

func (l *Loader) send(after time.Duration) {
	if l.isPreparedToSend() {
		return
	}
	l.sendLock.Lock()
	l.sending = true
	l.sendLock.Unlock()
	<-time.After(after)
	l.sendCh <- struct{}{}
	l.sendLock.Lock()
	l.sending = false
	l.sendLock.Unlock()
}

// Close sends the remaining entries to omega and sets the
// loader as closed
func (l *Loader) Close() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.closed {
		return nil
	}
	close(l.stopCh)
	l.closed = true
	return l.bulkLoad()
}

func (l *Loader) isClosed() bool {
	l.lock.RLock()
	closed := l.closed
	l.lock.RUnlock()
	return closed
}

func (l *Loader) isPreparedToSend() bool {
	l.sendLock.RLock()
	sending := l.sending
	l.sendLock.RUnlock()
	return sending
}
