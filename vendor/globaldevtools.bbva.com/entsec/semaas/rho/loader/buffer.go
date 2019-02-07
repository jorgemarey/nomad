package loader

import (
	"bytes"
	"encoding/gob"
	"sync"

	"globaldevtools.bbva.com/entsec/semaas/rho/api"
)

// SpanBuffer returns a buffer that if able to store omega logs
// and be cleaned after they are send.
type SpanBuffer struct {
	entries    []*api.Span
	size       int
	off        int
	cleanedOff int
	offSize    int

	resizeLength int
	encBuffer    *bytes.Buffer
	lock         sync.Mutex
}

// NewSpanBuffer returns a buffer with the provided resize length
func NewSpanBuffer(resizeLength int) *SpanBuffer {
	return &SpanBuffer{
		entries:      make([]*api.Span, 0),
		encBuffer:    &bytes.Buffer{},
		resizeLength: resizeLength,
	}
}

// Add inserts a new entry to the buffer for later
func (b *SpanBuffer) Add(entry *api.Span) error {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.entries = append(b.entries, entry)
	gob.NewEncoder(b.encBuffer).Encode(entry)
	b.size += b.encBuffer.Len()
	b.encBuffer.Reset()
	return nil
}

// Len returns the number of elements in the buffer
func (b *SpanBuffer) Len() int {
	b.lock.Lock()
	defer b.lock.Unlock()
	return len(b.entries) - b.cleanedOff
}

// Size returns an aproximate size in bytes for the buffer
func (b *SpanBuffer) Size() int {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.size
}

// Cap returns the current capacity of the buffer
func (b *SpanBuffer) Cap() int {
	b.lock.Lock()
	defer b.lock.Unlock()
	return cap(b.entries)
}

// Entries returns the current saved items on the buffer that weren't cleaned
func (b *SpanBuffer) Entries() []*api.Span {
	b.lock.Lock()
	defer b.lock.Unlock()
	l := len(b.entries) - b.cleanedOff
	r := make([]*api.Span, l, l)
	copy(r, b.entries[b.cleanedOff:])
	b.off = len(b.entries)
	b.offSize = b.size
	return r
}

// Clean removes the read elements of the buffer (those previously retourned by
// calling Entries())
func (b *SpanBuffer) Clean() {
	b.lock.Lock()
	defer b.lock.Unlock()
	entries := b.entries
	if cap(b.entries) > b.resizeLength {
		b.entries = make([]*api.Span, len(b.entries)-b.off, len(b.entries)-b.off)
		copy(b.entries, entries[b.off:])
		b.off = 0
	}
	b.cleanedOff = b.off
	b.size = b.size - b.offSize
	b.size = 0
}

// Reset empty the buffer completely
func (b *SpanBuffer) Reset() {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.entries = b.entries[:0]
	if cap(b.entries) > b.resizeLength {
		b.entries = make([]*api.Span, 0)
		b.off = 0
	}
	b.cleanedOff = b.off
	b.size = 0
}
