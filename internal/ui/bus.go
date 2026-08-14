package ui

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

const (
	busRingSize = 1024
	busSubBuf   = 64
	eventsPath  = "/api/v1/events"
)

// BusEvent is one fan-out message to SSE clients. Kind is the SSE event name
// (log, run, cleanup, ovn, ping). Cluster/template let the web client switch
// "tabs" without opening a new stream.
type BusEvent struct {
	Seq      uint64          `json:"seq"`
	At       time.Time       `json:"at"`
	Cluster  string          `json:"cluster,omitempty"`
	Template string          `json:"template,omitempty"`
	Kind     string          `json:"kind"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type busSub struct {
	id uint64
	ch chan BusEvent
}

// EventBus is an in-process collector: kube/job log lines land here, then
// relay downstream over SSE. One ring for reconnect replay; slow clients drop.
type EventBus struct {
	seq  atomic.Uint64
	subN atomic.Uint64

	mu   sync.RWMutex
	ring []BusEvent
	head int // next write index
	n    int // valid entries
	subs map[uint64]*busSub
}

func NewEventBus() *EventBus {
	return &EventBus{
		ring: make([]BusEvent, busRingSize),
		subs: map[uint64]*busSub{},
	}
}

func (b *EventBus) Seq() uint64 {
	if b == nil {
		return 0
	}
	return b.seq.Load()
}

func (b *EventBus) Publish(kind, cluster, template string, data any) uint64 {
	if b == nil {
		return 0
	}
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte(`{}`)
	}
	ev := BusEvent{
		Seq:      b.seq.Add(1),
		At:       time.Now(),
		Cluster:  cluster,
		Template: template,
		Kind:     kind,
		Data:     raw,
	}
	b.mu.Lock()
	b.ring[b.head] = ev
	b.head = (b.head + 1) % len(b.ring)
	if b.n < len(b.ring) {
		b.n++
	}
	subs := make([]*busSub, 0, len(b.subs))
	for _, sub := range b.subs {
		subs = append(subs, sub)
	}
	b.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.ch <- ev:
		default:
			// slow web client — drop rather than stall the job
		}
	}
	return ev.Seq
}

// Subscribe returns a live channel plus replay of events with Seq > after.
func (b *EventBus) Subscribe(after uint64) (id uint64, ch <-chan BusEvent, replay []BusEvent) {
	if b == nil {
		c := make(chan BusEvent)
		close(c)
		return 0, c, nil
	}
	id = b.subN.Add(1)
	out := make(chan BusEvent, busSubBuf)
	b.mu.Lock()
	replay = b.replayLocked(after)
	b.subs[id] = &busSub{id: id, ch: out}
	b.mu.Unlock()
	return id, out, replay
}

func (b *EventBus) Unsubscribe(id uint64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if sub, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(sub.ch)
	}
	b.mu.Unlock()
}

func (b *EventBus) replayLocked(after uint64) []BusEvent {
	if b.n == 0 {
		return nil
	}
	out := make([]BusEvent, 0, b.n)
	start := b.head - b.n
	if start < 0 {
		start += len(b.ring)
	}
	for i := 0; i < b.n; i++ {
		ev := b.ring[(start+i)%len(b.ring)]
		if ev.Seq > after {
			out = append(out, ev)
		}
	}
	return out
}
