package resilience

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
)

// State representa el estado del circuit breaker
type State int

const (
	StateClosed State = iota // Normal, permite requests
	StateOpen                // Falla, rechaza requests
	StateHalfOpen            // Probando recuperación
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implementa el patrón Circuit Breaker
type CircuitBreaker struct {
	name            string
	logger          *logger.Logger
	mu              sync.RWMutex
	state           State
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	lastStateChange time.Time

	// Configuración
	config *Config
}

// Config configuración del circuit breaker
type Config struct {
	MaxFailures         int           // Fallos consecutivos antes de abrir
	Timeout             time.Duration // Tiempo en estado open antes de half-open
	HalfOpenSuccesses   int           // Éxitos en half-open antes de cerrar
	HalfOpenMaxRequests int           // Max requests permitidos en half-open
	FailureThreshold    float64       // % fallos para abrir (0.0-1.0)
	SampleSize          int           // Tamaño de muestra para threshold
}

// DefaultConfig configuración por defecto
func DefaultConfig() *Config {
	return &Config{
		MaxFailures:         5,
		Timeout:             60 * time.Second,
		HalfOpenSuccesses:   2,
		HalfOpenMaxRequests: 3,
		FailureThreshold:    0.5, // 50%
		SampleSize:          10,
	}
}

// NewCircuitBreaker crea un nuevo circuit breaker
func NewCircuitBreaker(name string, log *logger.Logger, config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}

	return &CircuitBreaker{
		name:            name,
		logger:          log,
		state:           StateClosed,
		lastStateChange: time.Now(),
		config:          config,
	}
}

// Execute ejecuta una función protegida por el circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Verificar si se puede ejecutar
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	// Ejecutar función
	err := fn()

	// Registrar resultado
	cb.afterRequest(err)

	return err
}

// beforeRequest verifica si se puede hacer el request
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	currentState := cb.state

	switch currentState {
	case StateClosed:
		// Permitir request
		return nil

	case StateOpen:
		// Verificar si es tiempo de intentar recuperación
		if time.Since(cb.lastStateChange) >= cb.config.Timeout {
			cb.setState(StateHalfOpen)
			cb.logger.Info("Circuit breaker transitioning to half-open",
				"name", cb.name,
				"open_duration", time.Since(cb.lastStateChange),
			)
			return nil
		}

		// Todavía open, rechazar
		return ErrCircuitOpen

	case StateHalfOpen:
		// Limitar requests en half-open
		if cb.successCount+cb.failureCount >= cb.config.HalfOpenMaxRequests {
			return ErrTooManyRequests
		}
		return nil

	default:
		return nil
	}
}

// afterRequest registra el resultado del request
func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onFailure maneja un fallo
func (cb *CircuitBreaker) onFailure() {
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		// Si excede max failures, abrir circuito
		if cb.failureCount >= cb.config.MaxFailures {
			cb.setState(StateOpen)
			cb.logger.Warn("Circuit breaker opened",
				"name", cb.name,
				"failures", cb.failureCount,
			)
		}

	case StateHalfOpen:
		// Un solo fallo en half-open vuelve a open
		cb.setState(StateOpen)
		cb.logger.Warn("Circuit breaker re-opened from half-open",
			"name", cb.name,
		)
	}
}

// onSuccess maneja un éxito
func (cb *CircuitBreaker) onSuccess() {
	cb.successCount++

	switch cb.state {
	case StateClosed:
		// Resetear contador de fallos
		if cb.failureCount > 0 {
			cb.failureCount = 0
		}

	case StateHalfOpen:
		// Si alcanza successes necesarios, cerrar circuito
		if cb.successCount >= cb.config.HalfOpenSuccesses {
			cb.setState(StateClosed)
			cb.logger.Info("Circuit breaker closed from half-open",
				"name", cb.name,
				"successes", cb.successCount,
			)
		}
	}
}

// setState cambia el estado del circuit breaker
func (cb *CircuitBreaker) setState(newState State) {
	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Resetear contadores según estado
	switch newState {
	case StateClosed:
		cb.failureCount = 0
		cb.successCount = 0
	case StateOpen:
		cb.successCount = 0
	case StateHalfOpen:
		cb.failureCount = 0
		cb.successCount = 0
	}

	cb.logger.Info("Circuit breaker state changed",
		"name", cb.name,
		"old_state", oldState.String(),
		"new_state", newState.String(),
	)
}

// GetState retorna el estado actual
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetMetrics retorna métricas del circuit breaker
func (cb *CircuitBreaker) GetMetrics() *Metrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return &Metrics{
		Name:            cb.name,
		State:           cb.state.String(),
		FailureCount:    cb.failureCount,
		SuccessCount:    cb.successCount,
		LastFailureTime: cb.lastFailureTime,
		LastStateChange: cb.lastStateChange,
		TimeSinceOpen:   time.Since(cb.lastStateChange),
	}
}

// Metrics métricas del circuit breaker
type Metrics struct {
	Name            string
	State           string
	FailureCount    int
	SuccessCount    int
	LastFailureTime time.Time
	LastStateChange time.Time
	TimeSinceOpen   time.Duration
}

// Reset resetea el circuit breaker manualmente
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateClosed)
	cb.logger.Info("Circuit breaker manually reset", "name", cb.name)
}

// ForceOpen fuerza el circuito a open (para mantenimiento)
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateOpen)
	cb.logger.Warn("Circuit breaker manually opened", "name", cb.name)
}

// CircuitBreakerManager gestiona múltiples circuit breakers
type CircuitBreakerManager struct {
	logger   *logger.Logger
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager crea un nuevo manager
func NewCircuitBreakerManager(log *logger.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		logger:   log,
		breakers: make(map[string]*CircuitBreaker),
	}
}

// Get obtiene o crea un circuit breaker
func (cbm *CircuitBreakerManager) Get(name string, config *Config) *CircuitBreaker {
	cbm.mu.RLock()
	cb, exists := cbm.breakers[name]
	cbm.mu.RUnlock()

	if exists {
		return cb
	}

	// Crear nuevo circuit breaker
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	// Double-check después de adquirir lock
	if cb, exists := cbm.breakers[name]; exists {
		return cb
	}

	cb = NewCircuitBreaker(name, cbm.logger, config)
	cbm.breakers[name] = cb

	cbm.logger.Info("Created new circuit breaker", "name", name)

	return cb
}

// GetAll retorna todos los circuit breakers
func (cbm *CircuitBreakerManager) GetAll() map[string]*CircuitBreaker {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	// Copiar mapa
	breakers := make(map[string]*CircuitBreaker, len(cbm.breakers))
	for k, v := range cbm.breakers {
		breakers[k] = v
	}

	return breakers
}

// GetMetrics retorna métricas de todos los circuit breakers
func (cbm *CircuitBreakerManager) GetMetrics() map[string]*Metrics {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	metrics := make(map[string]*Metrics, len(cbm.breakers))
	for name, cb := range cbm.breakers {
		metrics[name] = cb.GetMetrics()
	}

	return metrics
}

// ResetAll resetea todos los circuit breakers
func (cbm *CircuitBreakerManager) ResetAll() {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	for _, cb := range cbm.breakers {
		cb.Reset()
	}

	cbm.logger.Info("Reset all circuit breakers")
}

// MonitorCircuitBreakers monitorea estado de circuit breakers
func (cbm *CircuitBreakerManager) MonitorCircuitBreakers(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := cbm.GetMetrics()

			openCount := 0
			halfOpenCount := 0

			for name, m := range metrics {
				if m.State == "open" {
					openCount++
					cbm.logger.Warn("Circuit breaker is open",
						"name", name,
						"failures", m.FailureCount,
						"open_duration", m.TimeSinceOpen,
					)
				} else if m.State == "half_open" {
					halfOpenCount++
				}
			}

			if openCount > 0 || halfOpenCount > 0 {
				cbm.logger.Info("Circuit breaker status",
					"open", openCount,
					"half_open", halfOpenCount,
					"total", len(metrics),
				)
			}
		}
	}
}

// Configuraciones específicas para servicios

// OpenAIConfig configuración para OpenAI API
func OpenAIConfig() *Config {
	return &Config{
		MaxFailures:         3, // Más estricto
		Timeout:             30 * time.Second,
		HalfOpenSuccesses:   2,
		HalfOpenMaxRequests: 2,
		FailureThreshold:    0.3, // 30%
		SampleSize:          10,
	}
}

// LibreOfficeConfig configuración para LibreOffice
func LibreOfficeConfig() *Config {
	return &Config{
		MaxFailures:         5,
		Timeout:             60 * time.Second,
		HalfOpenSuccesses:   3,
		HalfOpenMaxRequests: 5,
		FailureThreshold:    0.5, // 50%
		SampleSize:          10,
	}
}

// DatabaseConfig configuración para base de datos
func DatabaseConfig() *Config {
	return &Config{
		MaxFailures:         10, // Más tolerante
		Timeout:             30 * time.Second,
		HalfOpenSuccesses:   5,
		HalfOpenMaxRequests: 10,
		FailureThreshold:    0.7, // 70%
		SampleSize:          20,
	}
}

// RedisConfig configuración para Redis
func RedisConfig() *Config {
	return &Config{
		MaxFailures:         10,
		Timeout:             20 * time.Second,
		HalfOpenSuccesses:   3,
		HalfOpenMaxRequests: 5,
		FailureThreshold:    0.6, // 60%
		SampleSize:          15,
	}
}

// Example: Uso de Circuit Breaker con OpenAI

/*
func CallOpenAI(ctx context.Context, cb *CircuitBreaker, prompt string) (string, error) {
	var result string

	err := cb.Execute(ctx, func() error {
		// Llamada real a OpenAI
		resp, err := openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: "gpt-4-vision-preview",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: prompt,
				},
			},
		})

		if err != nil {
			return fmt.Errorf("OpenAI API error: %w", err)
		}

		result = resp.Choices[0].Message.Content
		return nil
	})

	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			// Circuito abierto, usar fallback
			return useFallbackOCR(ctx, prompt)
		}
		return "", err
	}

	return result, nil
}
*/

// RetryWithCircuitBreaker combina retry + circuit breaker
func RetryWithCircuitBreaker(
	ctx context.Context,
	cb *CircuitBreaker,
	fn func() error,
	maxRetries int,
	backoff time.Duration,
) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Ejecutar con circuit breaker
		err := cb.Execute(ctx, fn)

		if err == nil {
			return nil
		}

		lastErr = err

		// Si circuito está abierto, no reintentar
		if errors.Is(err, ErrCircuitOpen) {
			return err
		}

		// Backoff antes de reintentar
		if attempt < maxRetries {
			delay := backoff * time.Duration(1<<uint(attempt))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}
