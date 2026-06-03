package mount

import (
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func TestMountEventBusSubscribeUnsubscribe(t *testing.T) {
	eb := NewMountEventBus()
	ch := eb.Subscribe("test1")

	eb.Publish(&protocol.Event{
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

	eb.Unsubscribe("test1")

	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

func TestMountEventBusNoSubscriber(t *testing.T) {
	eb := NewMountEventBus()
	eb.Publish(&protocol.Event{
		Type: protocol.EventSyncCompleted,
	})
}

func TestMountEventBusMultipleSubscribers(t *testing.T) {
	eb := NewMountEventBus()
	ch1 := eb.Subscribe("a")
	ch2 := eb.Subscribe("b")

	eb.Publish(&protocol.Event{Type: protocol.EventMountStateChanged, Timestamp: 1})

	<-ch1
	<-ch2
}

func TestMountEventBusSlowSubscriber(t *testing.T) {
	eb := NewMountEventBus()
	_ = eb.Subscribe("slow")

	for i := 0; i < 200; i++ {
		eb.Publish(&protocol.Event{Type: protocol.EventMountStateChanged, Timestamp: int64(i)})
	}
}
