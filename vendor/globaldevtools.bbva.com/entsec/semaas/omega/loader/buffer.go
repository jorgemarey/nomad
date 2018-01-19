package loader

import (
	"bytes"
	"encoding/gob"
	"sync"

	"globaldevtools.bbva.com/entsec/semaas/omega/api"
)

// LogEntryBuffer returns a buffer that if able to store omega logs
// and be cleaned after they are send.
type LogEntryBuffer struct {
	entries    []*api.LogEntry
	size       int
	off        int
	cleanedOff int
	offSize    int

	resizeLength int
	encBuffer    *bytes.Buffer
	lock         sync.Mutex
}

func NewLogEntryBuffer(resizeLength int) *LogEntryBuffer {
	return &LogEntryBuffer{
		entries:      make([]*api.LogEntry, 0),
		encBuffer:    &bytes.Buffer{},
		resizeLength: resizeLength,
	}
}

func (b *LogEntryBuffer) Add(entry *api.LogEntry) error {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.entries = append(b.entries, entry)
	gob.NewEncoder(b.encBuffer).Encode(entry)
	b.size += b.encBuffer.Len()
	b.encBuffer.Reset()
	return nil
}

// Len returns the number of elements in the buffer
func (b *LogEntryBuffer) Len() int {
	b.lock.Lock()
	defer b.lock.Unlock()
	return len(b.entries) - b.cleanedOff
}

// Size returns an aproximate size in bytes for the buffer
func (b *LogEntryBuffer) Size() int {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.size
}

func (b *LogEntryBuffer) Cap() int {
	b.lock.Lock()
	defer b.lock.Unlock()
	return cap(b.entries)
}

func (b *LogEntryBuffer) Entries() []*api.LogEntry {
	b.lock.Lock()
	defer b.lock.Unlock()
	l := len(b.entries) - b.cleanedOff
	r := make([]*api.LogEntry, l, l)
	copy(r, b.entries[b.cleanedOff:])
	b.off = len(b.entries)
	b.offSize = b.size
	return r
}

// Clean removes the read elements of the buffer (those previously retourned by
// calling Entries())
func (b *LogEntryBuffer) Clean() {
	b.lock.Lock()
	defer b.lock.Unlock()
	entries := b.entries
	if cap(b.entries) > b.resizeLength {
		b.entries = make([]*api.LogEntry, len(b.entries)-b.off, len(b.entries)-b.off)
		copy(b.entries, entries[b.off:])
		b.off = 0
	}
	b.cleanedOff = b.off
	b.size = b.size - b.offSize
	b.size = 0
}

// Reset empty the buffer completely
func (b *LogEntryBuffer) Reset() {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.entries = b.entries[:0]
	if cap(b.entries) > b.resizeLength {
		b.entries = make([]*api.LogEntry, 0)
		b.off = 0
	}
	b.cleanedOff = b.off
	b.size = 0
}
