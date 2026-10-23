package telemetry

import "sync"

type Update struct {
	Signal       Signal `json:"signal"`
	ReceivedAtNS int64  `json:"received_at_ns"`
}

// ChangeBus coalesces notifications so a slow UI cannot grow an unbounded queue.
type ChangeBus struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uint64]chan Update
}

func NewChangeBus() *ChangeBus {
	return &ChangeBus{subscribers: make(map[uint64]chan Update)}
}

func (b *ChangeBus) Subscribe() (<-chan Update, func()) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	updates := make(chan Update, 1)
	b.subscribers[id] = updates
	b.mu.Unlock()

	var once sync.Once
	return updates, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, id)
			close(updates)
			b.mu.Unlock()
		})
	}
}

func (b *ChangeBus) Publish(update Update) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- update:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- update:
			default:
			}
		}
	}
}
