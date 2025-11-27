package utils

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// GoroutineLimiter limita el número de goroutines concurrentes
type GoroutineLimiter struct {
	maxGoroutines int
	semaphore     chan struct{}
	logger        *logger.Logger
	mu            sync.RWMutex
	active        int
}

// NewGoroutineLimiter crea un nuevo limitador de goroutines
func NewGoroutineLimiter(max int, log *logger.Logger) *GoroutineLimiter {
	return &GoroutineLimiter{
		maxGoroutines: max,
		semaphore:     make(chan struct{}, max),
		logger:        log,
		active:        0,
	}
}

// Go ejecuta una función en una goroutine limitada con timeout
func (gl *GoroutineLimiter) Go(ctx context.Context, fn func() error) error {
	select {
	case gl.semaphore <- struct{}{}:
		gl.mu.Lock()
		gl.active++
		current := gl.active
		gl.mu.Unlock()

		if current > gl.maxGoroutines*80/100 {
			gl.logger.Warn("High goroutine usage",
				"active", current,
				"max", gl.maxGoroutines,
				"percentage", (current*100)/gl.maxGoroutines,
			)
		}

		go func() {
			defer func() {
				<-gl.semaphore
				gl.mu.Lock()
				gl.active--
				gl.mu.Unlock()

				// Recover de panics
				if r := recover(); r != nil {
					gl.logger.Error("Goroutine panic recovered",
						"panic", r,
						"stack", string(debug.Stack()),
					)
				}
			}()

			// Ejecutar función con context
			if err := fn(); err != nil {
				gl.logger.Error("Goroutine task failed", "error", err)
			}
		}()

		return nil

	case <-ctx.Done():
		return fmt.Errorf("context cancelled before goroutine could start")

	default:
		return fmt.Errorf("goroutine limit reached: %d/%d active", gl.active, gl.maxGoroutines)
	}
}

// GoWithTimeout ejecuta una función con timeout específico
func (gl *GoroutineLimiter) GoWithTimeout(timeout time.Duration, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return gl.Go(ctx, fn)
}

// Active retorna el número de goroutines activas
func (gl *GoroutineLimiter) Active() int {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	return gl.active
}

// Available retorna el número de slots disponibles
func (gl *GoroutineLimiter) Available() int {
	gl.mu.RLock()
	defer gl.mu.RUnlock()
	return gl.maxGoroutines - gl.active
}

// Wait espera a que todas las goroutines terminen (con timeout)
func (gl *GoroutineLimiter) Wait(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		gl.mu.RLock()
		active := gl.active
		gl.mu.RUnlock()

		if active == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for goroutines: %d still active", active)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Stats retorna estadísticas del limiter
func (gl *GoroutineLimiter) Stats() map[string]interface{} {
	gl.mu.RLock()
	defer gl.mu.RUnlock()

	return map[string]interface{}{
		"max_goroutines": gl.maxGoroutines,
		"active":         gl.active,
		"available":      gl.maxGoroutines - gl.active,
		"usage_percent":  (gl.active * 100) / gl.maxGoroutines,
	}
}
