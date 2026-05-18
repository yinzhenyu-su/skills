package daemon

import (
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func TestEventManagerSubscribeUnsubscribe(t *testing.T) {
	em := NewEventManager()
	ch := em.Subscribe("test1")

	em.Publish(&protocol.Event{
		Type:      protocol.EventMountStateChanged,
		Timestamp: time.Now().UnixMilli(),
		Data:      "hello",
	})

	select {
	case evt := <-ch:
		if evt.Type != protocol.EventMountStateChanged {
			t.Fatalf("wrong event type: %s", evt.Type)
		}
	default:
		t.Fatal("expected event")
	}

	em.Unsubscribe("test1")

	// After unsubscribe, ch should be closed
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

func TestEventManagerNoSubscriber(t *testing.T) {
	em := NewEventManager()
	// Should not block or panic
	em.Publish(&protocol.Event{
		Type: protocol.EventSyncCompleted,
	})
}

func TestEventManagerMultipleSubscribers(t *testing.T) {
	em := NewEventManager()
	ch1 := em.Subscribe("a")
	ch2 := em.Subscribe("b")

	em.Publish(&protocol.Event{Type: protocol.EventMountStateChanged, Timestamp: 1})

	<-ch1
	<-ch2
}

func TestEventManagerSlowSubscriber(t *testing.T) {
	em := NewEventManager()
	_ = em.Subscribe("slow")

	// Fill the buffer
	for i := 0; i < 200; i++ {
		em.Publish(&protocol.Event{Type: protocol.EventMountStateChanged, Timestamp: int64(i)})
	}
	// Should not block - events dropped for slow subscriber
}
