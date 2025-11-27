package utils

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

func TestGoroutineLimiter_ConcurrentExecution(t *testing.T) {
	log := logger.New("info", "json")
	limiter := NewGoroutineLimiter(5, log)

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Intentar ejecutar 10 goroutines concurrentes (solo 5 deberían ejecutarse simultáneamente)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := limiter.Go(ctx, func() error {
				time.Sleep(100 * time.Millisecond)
				mu.Lock()
				successCount++
				mu.Unlock()
				return nil
			})

			if err != nil {
				t.Logf("Goroutine %d: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	if successCount == 0 {
		t.Error("No goroutines executed successfully")
	}

	t.Logf("Successfully executed %d goroutines", successCount)
}

func TestGoroutineLimiter_Timeout(t *testing.T) {
	log := logger.New("info", "json")
	limiter := NewGoroutineLimiter(2, log)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Llenar el limiter
	limiter.Go(context.Background(), func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})
	limiter.Go(context.Background(), func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	// Este debería fallar por timeout
	err := limiter.Go(ctx, func() error {
		return nil
	})

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	t.Logf("Correctly received timeout: %v", err)
}

func TestGoroutineLimiter_WaitCompletion(t *testing.T) {
	log := logger.New("info", "json")
	limiter := NewGoroutineLimiter(3, log)

	// Ejecutar 3 goroutines
	for i := 0; i < 3; i++ {
		limiter.Go(context.Background(), func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}

	// Esperar que todas terminen
	err := limiter.Wait(1 * time.Second)
	if err != nil {
		t.Errorf("Wait failed: %v", err)
	}

	if limiter.Active() != 0 {
		t.Errorf("Expected 0 active goroutines, got %d", limiter.Active())
	}
}

func TestGoroutineLimiter_Stats(t *testing.T) {
	log := logger.New("info", "json")
	limiter := NewGoroutineLimiter(10, log)

	// Ejecutar algunas goroutines
	limiter.Go(context.Background(), func() error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	limiter.Go(context.Background(), func() error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	stats := limiter.Stats()

	if stats["max_goroutines"].(int) != 10 {
		t.Errorf("Expected max_goroutines=10, got %d", stats["max_goroutines"])
	}

	if stats["available"].(int) > 10 {
		t.Errorf("Available goroutines should not exceed max")
	}

	t.Logf("Stats: %+v", stats)
}

func TestGoroutineLimiter_PanicRecovery(t *testing.T) {
	log := logger.New("info", "json")
	limiter := NewGoroutineLimiter(5, log)

	// Esta goroutine hará panic
	limiter.Go(context.Background(), func() error {
		panic("intentional panic for testing")
	})

	// Esperar a que se procese
	time.Sleep(100 * time.Millisecond)

	// El limiter debería seguir funcionando después del panic
	err := limiter.Go(context.Background(), func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Limiter should continue working after panic: %v", err)
	}
}

func BenchmarkGoroutineLimiter_Sequential(b *testing.B) {
	log := logger.New("error", "json") // Solo errores en benchmark
	limiter := NewGoroutineLimiter(100, log)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Go(context.Background(), func() error {
			time.Sleep(1 * time.Millisecond)
			return nil
		})
	}

	limiter.Wait(10 * time.Second)
}

func BenchmarkGoroutineLimiter_Parallel(b *testing.B) {
	log := logger.New("error", "json")
	limiter := NewGoroutineLimiter(100, log)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Go(context.Background(), func() error {
				time.Sleep(1 * time.Millisecond)
				return nil
			})
		}
	})

	limiter.Wait(10 * time.Second)
}
