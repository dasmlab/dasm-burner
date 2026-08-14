package ui

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventBusPublishSubscribeReplay(t *testing.T) {
	b := NewEventBus()
	seq := b.Publish("log", "TEST3", "smoke", logLine{Level: "info", Phase: "CLEANUP", Message: "start"})
	if seq != 1 {
		t.Fatalf("seq %d", seq)
	}
	id, ch, replay := b.Subscribe(0)
	defer b.Unsubscribe(id)
	if len(replay) != 1 {
		t.Fatalf("replay %d", len(replay))
	}
	if replay[0].Kind != "log" || replay[0].Cluster != "TEST3" {
		t.Fatalf("replay %+v", replay[0])
	}
	var line logLine
	if err := json.Unmarshal(replay[0].Data, &line); err != nil || line.Message != "start" {
		t.Fatalf("data %s err %v", replay[0].Data, err)
	}

	b.Publish("cleanup", "TEST3", "", map[string]any{"cleaning": true, "managedTotal": 9})
	select {
	case ev := <-ch:
		if ev.Kind != "cleanup" {
			t.Fatalf("got %s", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for live event")
	}

	id2, _, replay2 := b.Subscribe(seq)
	defer b.Unsubscribe(id2)
	if len(replay2) != 1 || replay2[0].Kind != "cleanup" {
		t.Fatalf("after-seq replay %+v", replay2)
	}
}

func TestEventBusSlowClientDrops(t *testing.T) {
	b := NewEventBus()
	id, ch, _ := b.Subscribe(0)
	defer b.Unsubscribe(id)
	for i := 0; i < busSubBuf+20; i++ {
		b.Publish("ping", "", "", map[string]int{"i": i})
	}
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			if n == 0 {
				t.Fatal("expected some events")
			}
			return
		}
	}
}
