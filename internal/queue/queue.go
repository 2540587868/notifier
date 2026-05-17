package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/ysqss/notifier/internal/message"
)

type DispatchTask struct {
	Message     *message.RenderedMessage
	ChannelName string
	Attempt     int
	MaxRetries  int
}

type Queue struct {
	highPriority   chan *DispatchTask
	normalPriority chan *DispatchTask
	dropped        atomic.Int64
	enqueued       atomic.Int64
	wg             sync.WaitGroup
	done           chan struct{}
}

func New(capacity int) *Queue {
	if capacity <= 0 {
		capacity = 4096
	}
	return &Queue{
		highPriority:   make(chan *DispatchTask, capacity),
		normalPriority: make(chan *DispatchTask, capacity),
		done:           make(chan struct{}),
	}
}

func (q *Queue) Enqueue(task *DispatchTask) error {
	var ch chan *DispatchTask
	if task.Message.Original.Level.Priority() <= 1 {
		ch = q.highPriority
	} else {
		ch = q.normalPriority
	}

	select {
	case ch <- task:
		q.enqueued.Add(1)
		return nil
	default:
		q.dropped.Add(1)
		slog.Warn("queue full, dropping task",
			"channel", task.ChannelName,
			"level", string(task.Message.Original.Level),
			"dropped_total", q.dropped.Load(),
		)
		return ErrQueueFull
	}
}

func (q *Queue) Start(workers int, handler func(context.Context, *DispatchTask)) {
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.consume(handler)
	}
}

func (q *Queue) consume(handler func(context.Context, *DispatchTask)) {
	defer q.wg.Done()
	ctx := context.Background()
	for {
		select {
		case task, ok := <-q.highPriority:
			if !ok {
				for t := range q.normalPriority {
					handler(ctx, t)
				}
				return
			}
			handler(ctx, task)
		default:
			select {
			case task, ok := <-q.highPriority:
				if !ok {
					return
				}
				handler(ctx, task)
			case task, ok := <-q.normalPriority:
				if !ok {
					return
				}
				handler(ctx, task)
			case <-q.done:
				return
			}
		}
	}
}

func (q *Queue) Shutdown() {
	close(q.done)
	close(q.highPriority)
	close(q.normalPriority)
	q.wg.Wait()
}

func (q *Queue) Dropped() int64 {
	return q.dropped.Load()
}

func (q *Queue) Enqueued() int64 {
	return q.enqueued.Load()
}

var ErrQueueFull = fmt.Errorf("queue full")
