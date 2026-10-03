package httptransport

import (
	"net/http"
	"sync"
)

// Hub wakes every open event stream after a change; clients refetch what they show.
// ponytail: every stream hears every change; filter per project if fleet refetches grow heavy.
type Hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{} // nil once closed
}

func NewHub() *Hub { return &Hub{subs: map[chan struct{}]struct{}{}} }

// Publish never blocks: a stream with a wake pending already covers this change.
func (h *Hub) Publish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Close ends every stream, so open tabs do not hold a graceful shutdown to its timeout.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		close(ch)
	}
	h.subs = nil
}

func (h *Hub) subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{} // a (re)connect may have missed changes, so the client refetches at once
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		close(ch)
		return ch, func() {}
	}
	h.subs[ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *Handler) streamEvents(w http.ResponseWriter, r *http.Request) {
	events, unsubscribe := h.events.subscribe()
	defer unsubscribe()
	sse(w, r, events, func(struct{}) any { return struct{}{} })
}
