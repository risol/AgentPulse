package ingest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

func TestQueueRejectsWhenBoundedBufferIsFull(t *testing.T) {
	writer := &blockingWriter{entered: make(chan struct{}, 2), release: make(chan struct{})}
	queue := NewQueue(writer, 1)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(writer.release) }) }
	defer func() {
		release()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = queue.Close(ctx)
	}()

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- queue.SaveTelemetry(context.Background(), telemetry.Batch{Signal: telemetry.SignalLogs})
	}()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("first batch never reached the writer")
	}
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- queue.SaveTelemetry(context.Background(), telemetry.Batch{Signal: telemetry.SignalMetrics})
	}()
	deadline := time.Now().Add(time.Second)
	for queue.Depth() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if queue.Depth() != 1 {
		t.Fatal("second batch did not occupy the bounded queue")
	}
	if err := queue.SaveTelemetry(context.Background(), telemetry.Batch{Signal: telemetry.SignalTraces}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("full queue returned %v, want ErrQueueFull", err)
	}
	release()
	for name, result := range map[string]<-chan error{"first": firstResult, "second": secondResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Errorf("%s batch write failed: %v", name, err)
			}
		case <-time.After(time.Second):
			t.Errorf("%s batch was not completed", name)
		}
	}
}

type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockingWriter) SaveTelemetry(context.Context, telemetry.Batch) error {
	w.entered <- struct{}{}
	<-w.release
	return nil
}
