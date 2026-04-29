package app

import "sync"

type Hub struct {
	mu        sync.Mutex
	subs      map[string]map[chan string]struct{}
	eventSubs map[chan string]struct{}
}

func NewHub() *Hub {
	return &Hub{
		subs:      map[string]map[chan string]struct{}{},
		eventSubs: map[chan string]struct{}{},
	}
}

func (h *Hub) Subscribe(taskID string) chan string {
	ch := make(chan string, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[taskID]; !ok {
		h.subs[taskID] = map[chan string]struct{}{}
	}
	h.subs[taskID][ch] = struct{}{}
	return ch
}

func (h *Hub) Unsubscribe(taskID string, ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.subs[taskID]; ok {
		delete(subs, ch)
		close(ch)
		if len(subs) == 0 {
			delete(h.subs, taskID)
		}
	}
}

func (h *Hub) Publish(taskID string, text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[taskID] {
		select {
		case ch <- text:
		default:
		}
	}
}

func (h *Hub) SubscribeEvents() chan string {
	ch := make(chan string, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.eventSubs[ch] = struct{}{}
	return ch
}

func (h *Hub) UnsubscribeEvents(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.eventSubs[ch]; ok {
		delete(h.eventSubs, ch)
		close(ch)
	}
}

func (h *Hub) PublishEvent(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.eventSubs {
		select {
		case ch <- text:
		default:
		}
	}
}
