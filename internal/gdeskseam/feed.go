package gdeskseam

import "sync"

// MaxPending bounds one connection's outbound queue.
//
// Sized for a stall, not for a backlog: a reply arrives in pieces of a few
// words, so this is several seconds of talking. Past it the connection is not
// keeping up, and holding more would trade memory for facts the glass is
// about to refetch anyway.
const MaxPending = 256

// Feed hands one log's events to every open connection.
//
// It exists because Log.Notify takes a single listener and must not block —
// it runs while the sequence number is still held, so a socket write there
// would stop the desk. The feed is the buffer that makes that safe.
type Feed struct {
	mu      sync.Mutex
	readers map[*Reader]struct{}
}

// NewFeed makes an empty feed.
func NewFeed() *Feed {
	return &Feed{readers: make(map[*Reader]struct{})}
}

// Publish is what LogConfig.Notify is set to. It never blocks.
func (f *Feed) Publish(event Event) {
	f.mu.Lock()
	readers := make([]*Reader, 0, len(f.readers))
	for reader := range f.readers {
		readers = append(readers, reader)
	}
	f.mu.Unlock()

	for _, reader := range readers {
		reader.offer(event)
	}
}

// Subscribe opens a queue for one connection. Close it when the connection
// goes away, or the feed keeps filling a queue nobody reads.
func (f *Feed) Subscribe() *Reader {
	reader := &Reader{feed: f, ready: make(chan struct{}, 1)}
	f.mu.Lock()
	f.readers[reader] = struct{}{}
	f.mu.Unlock()
	return reader
}

// Readers counts the open connections.
func (f *Feed) Readers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.readers)
}

func (f *Feed) drop(reader *Reader) {
	f.mu.Lock()
	delete(f.readers, reader)
	f.mu.Unlock()
}

// Reader is one connection's queue.
type Reader struct {
	feed    *Feed
	mu      sync.Mutex
	pending []Event
	ready   chan struct{}
	closed  bool
}

// Ready fires when there is something to drain. It is closed when the reader
// is, so a write pump selecting on it wakes up and stops.
func (r *Reader) Ready() <-chan struct{} {
	return r.ready
}

// Drain takes everything queued, oldest first.
func (r *Reader) Drain() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 {
		return nil
	}
	events := r.pending
	r.pending = nil
	return events
}

// Close stops the reader and unsubscribes it.
func (r *Reader) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.pending = nil
	close(r.ready)
	r.mu.Unlock()

	r.feed.drop(r)
}

// offer queues an event, or drops it.
//
// A full queue drops the newest and says nothing about it. The glass numbers
// what it has received and asks for the rest with stream.resync, so telling it
// separately would be a second mechanism for a problem it already solves — and
// the older events still queued stay contiguous, which keeps the gap at the
// end where the resync starts.
func (r *Reader) offer(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || len(r.pending) >= MaxPending {
		return
	}
	r.pending = append(r.pending, event)

	// Signalled while the lock is held, because Close closes this channel:
	// signalling outside would race a concurrent Close into a send on a closed
	// channel. The send is non-blocking, so holding the lock costs nothing.
	select {
	case r.ready <- struct{}{}:
	default:
		// Already awake. One signal is enough, because Drain takes everything.
	}
}
