package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// eventHub pushes messages to web pages as Server-Sent Events. Publish stores
// the message as the latest and sends it to every connected client; a client
// that connects later is sent the latest message straight away.
type eventHub struct {
	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	latest []byte
}

func newEventHub() *eventHub {
	return &eventHub{subs: map[chan []byte]struct{}{}}
}

// Publish stores msg as the latest message and sends it to every client.
func (h *eventHub) Publish(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latest = msg
	for ch := range h.subs {
		select {
		case ch <- msg:
		default: // client is not keeping up; it gets the next one
		}
	}
}

// Latest returns the most recently published message, or nil.
func (h *eventHub) Latest() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latest
}

// Clients returns the number of connected clients.
func (h *eventHub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func (h *eventHub) subscribe() (chan []byte, []byte) {
	ch := make(chan []byte, 4)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[ch] = struct{}{}
	return ch, h.latest
}

func (h *eventHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
}

// ServeHTTP streams the hub's messages as Server-Sent Events: the latest
// message as soon as the client connects, then each new one as it is
// published. Each event's data is one JSON message. A comment line is sent
// every 25 s so idle connections are not dropped by proxies.
func (h *eventHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming is not supported.", http.StatusInternalServerError)
		return
	}
	hdr := w.Header()
	hdr.Set("Access-Control-Allow-Origin", "*")
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")

	ch, latest := h.subscribe()
	defer h.unsubscribe(ch)

	send := func(msg []byte) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Ask the browser to wait 5 s before reconnecting, then send what we have.
	if _, err := fmt.Fprint(w, "retry: 5000\n\n"); err != nil {
		return
	}
	if latest != nil && !send(latest) {
		return
	}
	flusher.Flush()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if !send(msg) {
				return
			}
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
