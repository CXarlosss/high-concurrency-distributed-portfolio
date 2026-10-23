package game

import (
	"context"
	"sync/atomic"
	"time"
)

type BroadcastFunc func(serverTick uint32)

type Engine struct {
	world          *World
	ctx            context.Context
	cancel         context.CancelFunc
	onTickComplete BroadcastFunc

	// Telemetry metrics
	lastTickDurationNs int64
	totalTicksExecuted uint64
}

func NewEngine(w *World, onTick BroadcastFunc) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		world:          w,
		ctx:            ctx,
		cancel:         cancel,
		onTickComplete: onTick,
	}
}

func (e *Engine) LastTickDurationNs() int64 {
	return atomic.LoadInt64(&e.lastTickDurationNs)
}

func (e *Engine) TotalTicks() uint64 {
	return atomic.LoadUint64(&e.totalTicksExecuted)
}

func (e *Engine) Stop() {
	e.cancel()
}

func (e *Engine) RunLoop() {
	ticker := time.NewTicker(time.Second / TargetTPS)
	defer ticker.Stop()

	previousTime := time.Now()
	var accumulator time.Duration
	tickDuration := time.Second / TargetTPS

	for {
		select {
		case <-e.ctx.Done():
			return
		case currentTime := <-ticker.C:
			frameTime := currentTime.Sub(previousTime)
			previousTime = currentTime

			if frameTime > 250*time.Millisecond {
				frameTime = 250 * time.Millisecond
			}

			accumulator += frameTime

			for accumulator >= tickDuration {
				start := time.Now()
				e.world.Update(FixedDeltaTime)
				duration := time.Since(start).Nanoseconds()

				atomic.StoreInt64(&e.lastTickDurationNs, duration)
				atomic.AddUint64(&e.totalTicksExecuted, 1)

				if e.onTickComplete != nil {
					e.onTickComplete(e.world.TickCount())
				}

				accumulator -= tickDuration
			}
		}
	}
}
