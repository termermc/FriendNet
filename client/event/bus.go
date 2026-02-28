package event

import (
	"slices"
	"sync"

	"friendnet.org/common"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
)

// SubscriberFunc is a function that handles new events.
// It is run in its own goroutine.
type SubscriberFunc func(event *v1.Event, ctx *v1.EventContext)

// SubscriptionId is an identifier for an event subscription.
// It is used to unsubscribe.
type SubscriptionId struct {
	string
}

type subscription struct {
	id SubscriptionId
	fn SubscriberFunc
}

// Bus is an event bus.
// Events can be published and subscribed to.
type Bus struct {
	mu sync.RWMutex

	subscriptions []subscription
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		subscriptions: make([]subscription, 0),
	}
}

// Subscribe adds a new event subscription.
// The passed function will be called in its own goroutine for each new event.
func (b *Bus) Subscribe(fn SubscriberFunc) SubscriptionId {
	id := SubscriptionId{common.RandomB64UrlStr(4)}

	b.mu.Lock()
	b.subscriptions = append(b.subscriptions, subscription{
		id: id,
		fn: fn,
	})
	b.mu.Unlock()

	return id
}

// Unsubscribe removes an event subscription.
func (b *Bus) Unsubscribe(id SubscriptionId) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscriptions = slices.DeleteFunc(b.subscriptions, func(sub subscription) bool {
		return sub.id != id
	})
}

// CreatePublisher creates a new publisher for the bus with the specified event context.
func (b *Bus) CreatePublisher(eventCtx *v1.EventContext) *Publisher {
	return &Publisher{
		bus:      b,
		eventCtx: eventCtx,
	}
}

// Publisher is an event publisher for a Bus.
// It publishes events using its context.
type Publisher struct {
	bus      *Bus
	eventCtx *v1.EventContext
}

// Publish publishes a new event to all subscribers.
func (p *Publisher) Publish(event *v1.Event) {
	p.bus.mu.RLock()
	defer p.bus.mu.RUnlock()

	for _, sub := range p.bus.subscriptions {
		go sub.fn(event, p.eventCtx)
	}
}
