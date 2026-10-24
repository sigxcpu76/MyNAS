package application

import (
	"context"
	"sync"

	"github.com/sigxcpu/nas/internal/domain"
)

// EventBus provides thread-safe pub/sub dispatch for system notification events.
type EventBus struct {
	mu          sync.RWMutex
	closed      bool
	subscribers map[chan domain.Event]*sync.Once
}

// NewEventBus constructs a new EventBus instance.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan domain.Event]*sync.Once),
	}
}

// Publish broadcasts a domain event to all active subscribers.
func (b *EventBus) Publish(evt domain.Event) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}

	for ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
			// Non-blocking drop if a consumer channel buffer is full
		}
	}
}

// Subscribe registers a new subscriber channel that will receive domain events.
// The channel is closed automatically when ctx is cancelled or EventBus is closed.
func (b *EventBus) Subscribe(ctx context.Context) <-chan domain.Event {
	ch := make(chan domain.Event, 64)
	if b == nil {
		close(ch)
		return ch
	}

	once := &sync.Once{}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch
	}
	b.subscribers[ch] = once
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
		once.Do(func() {
			close(ch)
		})
	}()

	return ch
}

// Close terminates all active subscriber channels and clears the EventBus.
func (b *EventBus) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := b.subscribers
	b.subscribers = make(map[chan domain.Event]*sync.Once)
	b.mu.Unlock()

	for ch, once := range subs {
		once.Do(func() {
			close(ch)
		})
	}
}
