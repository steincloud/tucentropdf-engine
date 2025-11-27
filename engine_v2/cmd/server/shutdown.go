package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// ShutdownManager gestiona el cierre graceful del servidor
type ShutdownManager struct {
	logger            *logger.Logger
	app               *fiber.App
	shutdownTimeout   time.Duration
	drainTimeout      time.Duration
	shutdownCallbacks []func(context.Context) error
	mu                sync.Mutex
	isShuttingDown    bool
}

// NewShutdownManager crea un nuevo gestor de shutdown
func NewShutdownManager(log *logger.Logger, app *fiber.App) *ShutdownManager {
	return &ShutdownManager{
		logger:            log,
		app:               app,
		shutdownTimeout:   30 * time.Second,
		drainTimeout:      10 * time.Second,
		shutdownCallbacks: make([]func(context.Context) error, 0),
	}
}

// RegisterShutdownCallback registra una función a ejecutar en shutdown
func (sm *ShutdownManager) RegisterShutdownCallback(fn func(context.Context) error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.shutdownCallbacks = append(sm.shutdownCallbacks, fn)
}

// ListenForSignals escucha señales del sistema operativo
func (sm *ShutdownManager) ListenForSignals(ctx context.Context) {
	sigChan := make(chan os.Signal, 1)

	// Escuchar SIGINT (Ctrl+C) y SIGTERM (kill)
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	// Bloquear hasta recibir señal
	sig := <-sigChan

	sm.logger.Info("Received shutdown signal", "signal", sig.String())

	// Iniciar shutdown graceful
	if err := sm.Shutdown(ctx); err != nil {
		sm.logger.Error("Shutdown failed", "error", err)
		os.Exit(1)
	}

	sm.logger.Info("Server shutdown completed successfully")
	os.Exit(0)
}

// Shutdown realiza el cierre graceful
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	sm.mu.Lock()
	if sm.isShuttingDown {
		sm.mu.Unlock()
		return errors.New("shutdown already in progress")
	}
	sm.isShuttingDown = true
	sm.mu.Unlock()

	sm.logger.Info("Starting graceful shutdown")

	// Context con timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, sm.shutdownTimeout)
	defer cancel()

	// Fase 1: Detener aceptar nuevas conexiones
	sm.logger.Info("Phase 1: Stopping new connections")
	if err := sm.stopAcceptingConnections(shutdownCtx); err != nil {
		sm.logger.Error("Failed to stop accepting connections", "error", err)
	}

	// Fase 2: Drenar conexiones existentes
	sm.logger.Info("Phase 2: Draining existing connections", "timeout", sm.drainTimeout)
	if err := sm.drainConnections(shutdownCtx); err != nil {
		sm.logger.Warn("Connection drain incomplete", "error", err)
	}

	// Fase 3: Ejecutar callbacks de shutdown
	sm.logger.Info("Phase 3: Running shutdown callbacks", "count", len(sm.shutdownCallbacks))
	if err := sm.runShutdownCallbacks(shutdownCtx); err != nil {
		sm.logger.Error("Shutdown callbacks failed", "error", err)
		return err
	}

	// Fase 4: Cerrar servidor HTTP
	sm.logger.Info("Phase 4: Shutting down HTTP server")
	if err := sm.app.Shutdown(); err != nil {
		sm.logger.Error("Failed to shutdown HTTP server", "error", err)
		return err
	}

	sm.logger.Info("Graceful shutdown completed")
	return nil
}

// stopAcceptingConnections detiene aceptar nuevas conexiones
func (sm *ShutdownManager) stopAcceptingConnections(ctx context.Context) error {
	// Fiber no tiene API específica para esto, pero podemos
	// usar un middleware que rechace nuevos requests
	sm.app.Use(func(c *fiber.Ctx) error {
		if sm.isShuttingDown {
			c.Set("Connection", "close")
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    "SERVER_SHUTTING_DOWN",
					"message": "Server is shutting down, please retry",
				},
			})
		}
		return c.Next()
	})

	return nil
}

// drainConnections espera a que conexiones existentes terminen
func (sm *ShutdownManager) drainConnections(ctx context.Context) error {
	drainCtx, cancel := context.WithTimeout(ctx, sm.drainTimeout)
	defer cancel()

	// Esperar con polling
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-drainCtx.Done():
			return drainCtx.Err()
		case <-ticker.C:
			// TODO: Implementar conteo real de conexiones activas
			// Por ahora asumimos que Fiber maneja esto internamente
			
			// Si no hay conexiones activas, retornar
			activeConnections := sm.getActiveConnections()
			if activeConnections == 0 {
				sm.logger.Info("All connections drained")
				return nil
			}

			sm.logger.Debug("Waiting for connections to drain", "active", activeConnections)
		}
	}
}

// getActiveConnections retorna el número de conexiones activas
func (sm *ShutdownManager) getActiveConnections() int {
	// TODO: Implementar conteo real de conexiones
	// Por ahora placeholder
	return 0
}

// runShutdownCallbacks ejecuta todos los callbacks registrados
func (sm *ShutdownManager) runShutdownCallbacks(ctx context.Context) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(sm.shutdownCallbacks))

	for i, callback := range sm.shutdownCallbacks {
		wg.Add(1)
		go func(idx int, fn func(context.Context) error) {
			defer wg.Done()

			sm.logger.Debug("Running shutdown callback", "index", idx)

			if err := fn(ctx); err != nil {
				sm.logger.Error("Shutdown callback failed", "index", idx, "error", err)
				errChan <- fmt.Errorf("callback %d: %w", idx, err)
				return
			}

			sm.logger.Debug("Shutdown callback completed", "index", idx)
		}(i, callback)
	}

	// Esperar a que todos terminen
	wg.Wait()
	close(errChan)

	// Recolectar errores
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown callbacks failed: %v", errors)
	}

	return nil
}

// ShutdownWithTimeout cierre con timeout personalizado
func (sm *ShutdownManager) ShutdownWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return sm.Shutdown(ctx)
}

// IsShuttingDown verifica si el servidor está en proceso de cierre
func (sm *ShutdownManager) IsShuttingDown() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.isShuttingDown
}

// ShutdownMiddleware middleware que verifica si el servidor está cerrando
func (sm *ShutdownManager) ShutdownMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if sm.IsShuttingDown() {
			c.Set("Connection", "close")
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    "SERVER_SHUTTING_DOWN",
					"message": "Server is shutting down, please retry",
					"retry_after": 10,
				},
			})
		}
		return c.Next()
	}
}

// Example: Shutdown callbacks para diferentes recursos

// DatabaseShutdownCallback cierra conexiones de base de datos
func DatabaseShutdownCallback(db interface{}) func(context.Context) error {
	return func(ctx context.Context) error {
		// TODO: Implementar cierre de DB
		// db.Close()
		return nil
	}
}

// RedisShutdownCallback cierra conexiones de Redis
func RedisShutdownCallback(redis interface{}) func(context.Context) error {
	return func(ctx context.Context) error {
		// TODO: Implementar cierre de Redis
		// redis.Close()
		return nil
	}
}

// WorkerShutdownCallback detiene workers background
func WorkerShutdownCallback(workers []interface{}) func(context.Context) error {
	return func(ctx context.Context) error {
		// Detener todos los workers
		for _, worker := range workers {
			// TODO: worker.Stop()
		}
		return nil
	}
}

// QueueShutdownCallback procesa cola antes de cerrar
func QueueShutdownCallback(queue interface{}) func(context.Context) error {
	return func(ctx context.Context) error {
		// TODO: Procesar jobs pendientes o moverlos a DLQ
		return nil
	}
}

// CacheShutdownCallback flush cache antes de cerrar
func CacheShutdownCallback(cache interface{}) func(context.Context) error {
	return func(ctx context.Context) error {
		// TODO: Flush cache a disco si es necesario
		return nil
	}
}

// FileCleanupShutdownCallback limpia archivos temporales
func FileCleanupShutdownCallback(tempDir string) func(context.Context) error {
	return func(ctx context.Context) error {
		// TODO: Limpiar archivos temporales
		return nil
	}
}

// MetricsShutdownCallback flush métricas finales
func MetricsShutdownCallback(metrics interface{}) func(context.Context) error {
	return func(ctx context.Context) error {
		// TODO: Push métricas finales a Prometheus
		return nil
	}
}

// Example: Uso del ShutdownManager

/*
func main() {
	log := logger.New()
	app := fiber.New()

	// Crear shutdown manager
	shutdownManager := NewShutdownManager(log, app)

	// Registrar callbacks
	shutdownManager.RegisterShutdownCallback(DatabaseShutdownCallback(db))
	shutdownManager.RegisterShutdownCallback(RedisShutdownCallback(redis))
	shutdownManager.RegisterShutdownCallback(WorkerShutdownCallback(workers))
	shutdownManager.RegisterShutdownCallback(QueueShutdownCallback(queue))

	// Añadir middleware de shutdown
	app.Use(shutdownManager.ShutdownMiddleware())

	// Iniciar servidor en goroutine
	go func() {
		if err := app.Listen(":8080"); err != nil {
			log.Error("Failed to start server", "error", err)
		}
	}()

	// Escuchar señales de cierre
	shutdownManager.ListenForSignals(context.Background())
}
*/

// ShutdownConfig configuración de shutdown
type ShutdownConfig struct {
	ShutdownTimeout time.Duration
	DrainTimeout    time.Duration
	EnableDrain     bool
	ForceTimeout    time.Duration
}

// DefaultShutdownConfig configuración por defecto
func DefaultShutdownConfig() *ShutdownConfig {
	return &ShutdownConfig{
		ShutdownTimeout: 30 * time.Second,
		DrainTimeout:    10 * time.Second,
		EnableDrain:     true,
		ForceTimeout:    35 * time.Second, // Ligeramente mayor que ShutdownTimeout
	}
}

// NewShutdownManagerWithConfig crea manager con configuración custom
func NewShutdownManagerWithConfig(log *logger.Logger, app *fiber.App, config *ShutdownConfig) *ShutdownManager {
	if config == nil {
		config = DefaultShutdownConfig()
	}

	return &ShutdownManager{
		logger:            log,
		app:               app,
		shutdownTimeout:   config.ShutdownTimeout,
		drainTimeout:      config.DrainTimeout,
		shutdownCallbacks: make([]func(context.Context) error, 0),
	}
}

// HealthCheckDuringShutdown retorna health check especial durante shutdown
func (sm *ShutdownManager) HealthCheckDuringShutdown() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if sm.IsShuttingDown() {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "shutting_down",
				"ready":  false,
				"message": "Server is shutting down gracefully",
			})
		}

		return c.Status(http.StatusOK).JSON(fiber.Map{
			"status": "healthy",
			"ready":  true,
		})
	}
}
