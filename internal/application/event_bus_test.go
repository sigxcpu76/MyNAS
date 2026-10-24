package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestEventBus_PublishAndSubscribe(t *testing.T) {
	bus := application.NewEventBus()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub1 := bus.Subscribe(ctx)
	sub2 := bus.Subscribe(ctx)

	evt := domain.NewEvent("docker", "stop", "dockerContainers/my-container", "Container stopped")
	evt.Payload = map[string]any{"container": "my-container"}

	bus.Publish(evt)

	select {
	case received := <-sub1:
		if received.Topic != "docker" || received.Action != "stop" || received.Resource != "dockerContainers/my-container" {
			t.Errorf("sub1 unexpected event: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sub1 timeout waiting for event")
	}

	select {
	case received := <-sub2:
		if received.Topic != "docker" || received.Action != "stop" || received.Resource != "dockerContainers/my-container" {
			t.Errorf("sub2 unexpected event: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sub2 timeout waiting for event")
	}

	// Cancel context and verify channels are closed
	cancel()
	time.Sleep(50 * time.Millisecond)

	select {
	case _, ok := <-sub1:
		if ok {
			t.Errorf("expected sub1 to be closed")
		}
	default:
		// drained
	}

	select {
	case _, ok := <-sub2:
		if ok {
			t.Errorf("expected sub2 to be closed")
		}
	default:
		// drained
	}
}

func TestEventBus_CloseIdempotencyAndRace(t *testing.T) {
	for i := 0; i < 50; i++ {
		bus := application.NewEventBus()
		ctx, cancel := context.WithCancel(context.Background())

		var subs []<-chan domain.Event
		for j := 0; j < 20; j++ {
			subs = append(subs, bus.Subscribe(ctx))
		}

		// Concurrently publish, cancel context, and close bus
		go func() {
			bus.Publish(domain.NewEvent("test", "test", "test", "test"))
		}()
		go func() {
			cancel()
		}()
		go func() {
			bus.Close()
		}()
		go func() {
			bus.Close() // double close test
		}()

		// All subs should be closed without panic
		for _, s := range subs {
			for range s {
			}
		}
	}
}
