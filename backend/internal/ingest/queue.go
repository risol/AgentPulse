// Package ingest owns the bounded write queue between decoded telemetry and
// persistence. Requests wait for the storage result so the OTLP acknowledgement
// continues to mean that the batch was committed.
package ingest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
)

var (
	ErrQueueFull = errors.New("telemetry write queue is full")
	ErrClosed    = errors.New("telemetry write queue is closed")
)

type Writer interface {
	SaveTelemetry(context.Context, telemetry.Batch) error
}

type writeRequest struct {
	ctx    context.Context
	batch  telemetry.Batch
	result chan error
}

type Queue struct {
	writer   Writer
	requests chan writeRequest
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
	rejected atomic.Uint64
}

func NewQueue(writer Writer, capacity int) *Queue {
	if capacity < 1 {
		capacity = 1
	}
	queue := &Queue{
		writer: writer, requests: make(chan writeRequest, capacity),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go queue.run()
	return queue
}

func (q *Queue) SaveTelemetry(ctx context.Context, batch telemetry.Batch) error {
	request := writeRequest{ctx: ctx, batch: batch, result: make(chan error, 1)}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrClosed
	}
	select {
	case q.requests <- request:
		q.mu.Unlock()
	default:
		q.rejected.Add(1)
		q.mu.Unlock()
		return ErrQueueFull
	}
	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-q.done:
		select {
		case err := <-request.result:
			return err
		default:
			return ErrClosed
		}
	}
}

func (q *Queue) Depth() int { return len(q.requests) }

func (q *Queue) Capacity() int { return cap(q.requests) }

func (q *Queue) Rejected() uint64 { return q.rejected.Load() }

func (q *Queue) Close(ctx context.Context) error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.stop)
	}
	q.mu.Unlock()
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Queue) run() {
	defer close(q.done)
	for {
		select {
		case request := <-q.requests:
			q.process(request)
		case <-q.stop:
			for {
				select {
				case request := <-q.requests:
					q.process(request)
				default:
					return
				}
			}
		}
	}
}

func (q *Queue) process(request writeRequest) {
	if err := request.ctx.Err(); err != nil {
		request.result <- err
		return
	}
	request.result <- q.writer.SaveTelemetry(request.ctx, request.batch)
}
