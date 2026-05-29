package seat

import "sync"

type StreamEvent struct {
	Type   string `json:"type"`
	SeatID *int64 `json:"seat_id,omitempty"`
	State  string `json:"state,omitempty"`
}

type Broker struct {
	mu          sync.Mutex
	subscribers map[chan StreamEvent]struct{}
}

func NewBroker() *Broker {
	return &Broker{subscribers: map[chan StreamEvent]struct{}{}}
}

func (b *Broker) Subscribe() (<-chan StreamEvent, func()) {
	ch := make(chan StreamEvent, 8)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, unsubscribe
}

func (b *Broker) Publish(event StreamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}
