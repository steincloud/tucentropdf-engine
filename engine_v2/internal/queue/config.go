package queue

import (
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/tucentropdf/engine-v2/internal/config"
)

// Config configuración para la cola de trabajos
type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	
	// Concurrency por worker
	OCRConcurrency    int
	OfficeConcurrency int
	
	// Prioridades
	CriticalPriority int // Pro users
	HighPriority     int // Premium users
	DefaultPriority  int // Free users
	
	// Retry
	MaxRetries int
	
	// Queues
	OCRQueue    string
	OfficeQueue string
	DefaultQueue string
}

// LoadConfig carga configuración desde config principal
func LoadConfig(cfg *config.Config) *Config {
	return &Config{
		RedisAddr:     fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
		RedisPassword: cfg.Redis.Password,
		RedisDB:       cfg.Redis.DB,
		
		// Concurrency por defecto
		OCRConcurrency:    3,  // 3 OCR jobs simultáneos
		OfficeConcurrency: 5,  // 5 Office conversions simultáneas
		
		// Prioridades (menor número = mayor prioridad)
		CriticalPriority: 1,  // Pro
		HighPriority:     5,  // Premium
		DefaultPriority:  10, // Free
		
		// Retry automático
		MaxRetries: 3,
		
		// Nombres de colas
		OCRQueue:     "ocr",
		OfficeQueue:  "office",
		DefaultQueue: "default",
	}
}

// NewClient crea un cliente Asynq para encolar jobs
func NewClient(cfg *Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
}

// NewOCRServer crea un servidor Asynq para procesar jobs de OCR
func NewOCRServer(cfg *Config) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		asynq.Config{
			Concurrency: cfg.OCRConcurrency,
			Queues: map[string]int{
				cfg.OCRQueue:     6, // 60% de capacidad dedicada a OCR
				cfg.DefaultQueue: 4, // 40% para otros jobs
			},
			StrictPriority: false, // Permite intercalar jobs de diferentes prioridades
			LogLevel:       asynq.InfoLevel,
		},
	)
}

// NewOfficeServer crea un servidor Asynq para procesar jobs de Office
func NewOfficeServer(cfg *Config) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		asynq.Config{
			Concurrency: cfg.OfficeConcurrency,
			Queues: map[string]int{
				cfg.OfficeQueue:  6, // 60% de capacidad dedicada a Office
				cfg.DefaultQueue: 4, // 40% para otros jobs
			},
			StrictPriority: false,
			LogLevel:       asynq.InfoLevel,
		},
	)
}

// GetPriorityForPlan retorna la prioridad según el plan del usuario
func (c *Config) GetPriorityForPlan(plan string) int {
	switch plan {
	case "pro":
		return c.CriticalPriority
	case "premium":
		return c.HighPriority
	default:
		return c.DefaultPriority
	}
}

// GetQueueForTask retorna el nombre de la cola según el tipo de tarea
func (c *Config) GetQueueForTask(taskType string) string {
	switch taskType {
	case "ocr", "ocr-classic", "ocr-ai":
		return c.OCRQueue
	case "office", "office-to-pdf":
		return c.OfficeQueue
	default:
		return c.DefaultQueue
	}
}
