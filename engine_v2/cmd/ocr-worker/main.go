package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/metrics"
	"github.com/tucentropdf/engine-v2/internal/ocr"
	"github.com/tucentropdf/engine-v2/internal/queue"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

func main() {
	// Cargar configuración
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Inicializar logger
	logger := logger.New(cfg.Log.Level, cfg.Log.Format)
	defer logger.Sync()

	logger.Info("🔍 Starting OCR Worker",
		"version", "2.0.0",
		"tesseract_enabled", true,
		"paddle_enabled", cfg.OCR.PaddleEnabled,
		"ai_enabled", cfg.AI.Enabled,
	)

	// Inicializar servicios
	storageService := storage.NewService(cfg, logger)
	
	// Crear servicio OCR Classic (Tesseract)
	ocrClassic, err := ocr.NewClassicService(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize Tesseract OCR", "error", err)
	}

	// Crear servicio OCR AI (OpenAI Vision)
	var ocrAI ocr.Service
	if cfg.AI.Enabled && cfg.AI.APIKey != "" {
		ocrAI = ocr.NewAIService(cfg, logger)
		logger.Info("✅ AI OCR enabled", "model", cfg.AI.Model)
	} else {
		logger.Warn("⚠️ AI OCR disabled - OPENAI_API_KEY not configured")
	}

	// Configurar cola
	queueConfig := queue.LoadConfig(cfg)
	server := queue.NewOCRServer(queueConfig)

	// Crear handler processor
	handler := &OCRHandler{
		logger:         logger,
		config:         cfg,
		storageService: storageService,
		ocrClassic:     ocrClassic,
		ocrAI:          ocrAI,
	}

	// Registrar task handlers
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeOCRClassic, handler.HandleOCRClassic)
	mux.HandleFunc(queue.TypeOCRAI, handler.HandleOCRAI)

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	// Iniciar servidor en goroutine
	// Registrar métricas de worker
	metrics.SetWorkerHealth("ocr", "default", true)
	metrics.SetWorkerConcurrency("ocr", queueConfig.OCRConcurrency)

	go func() {
		logger.Info("🚀 OCR Worker started", "concurrency", queueConfig.OCRConcurrency)
		if err := server.Run(mux); err != nil {
			logger.Error("OCR Worker failed", "error", err)
			metrics.SetWorkerHealth("ocr", "default", false)
			metrics.RecordWorkerRestart("ocr", "error")
		}
	}()

	// Esperar señal de shutdown
	<-done
	logger.Info("🛑 Shutting down OCR Worker...")

	// Shutdown graceful (30 segundos)
	server.Shutdown()
	logger.Info("✅ OCR Worker stopped")
}

// OCRHandler maneja procesamiento de jobs de OCR
type OCRHandler struct {
	logger         *logger.Logger
	config         *config.Config
	storageService storage.Service
	ocrClassic     ocr.Service
	ocrAI          ocr.Service
}

// HandleOCRClassic procesa jobs de OCR clásico (Tesseract)
func (h *OCRHandler) HandleOCRClassic(ctx context.Context, task *asynq.Task) error {
	start := time.Now()

	// Parsear payload
	var payload queue.OCRJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		h.logger.Error("Failed to unmarshal OCR payload", "error", err)
		return fmt.Errorf("invalid payload: %w", err)
	}

	h.logger.Info("Processing OCR Classic job",
		"job_id", payload.JobID,
		"user_id", payload.UserID,
		"file_path", payload.FilePath,
		"language", payload.Language,
	)

	// Verificar que el servicio esté disponible
	if h.ocrClassic == nil {
		return fmt.Errorf("Tesseract OCR service not available")
	}

	// Procesar OCR
	result, err := h.ocrClassic.ExtractText(ctx, payload.FilePath, payload.Language)
	if err != nil {
		h.logger.Error("OCR Classic processing failed",
			"job_id", payload.JobID,
			"error", err,
		)
		// Registrar métrica de error
		metrics.RecordWorkerJobProcessed("ocr", "failed", time.Since(start).Seconds(), "classic")
		metrics.RecordWorkerError("ocr", "processing_error")
		return fmt.Errorf("OCR processing failed: %w", err)
	}

	// Guardar resultado
	resultPath := fmt.Sprintf("%s_result.txt", payload.FilePath)
	if err := os.WriteFile(resultPath, []byte(result.Text), 0644); err != nil {
		return fmt.Errorf("failed to save result: %w", err)
	}

	duration := time.Since(start)
	h.logger.Info("OCR Classic job completed",
		"job_id", payload.JobID,
		"duration", duration,
		"text_length", len(result.Text),
		"confidence", result.Confidence,
	)

	// Registrar métricas de éxito
	metrics.RecordWorkerJobProcessed("ocr", "success", duration.Seconds(), "classic")
	metrics.RecordJobCompleted(queue.TypeOCRClassic, payload.Plan, duration.Seconds())
	metrics.RecordJobResultSize(queue.TypeOCRClassic, int64(len(result.Text)))

	return nil
}

// HandleOCRAI procesa jobs de OCR con IA (OpenAI Vision)
func (h *OCRHandler) HandleOCRAI(ctx context.Context, task *asynq.Task) error {
	start := time.Now()

	// Parsear payload
	var payload queue.OCRJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		h.logger.Error("Failed to unmarshal OCR AI payload", "error", err)
		return fmt.Errorf("invalid payload: %w", err)
	}

	h.logger.Info("Processing OCR AI job",
		"job_id", payload.JobID,
		"user_id", payload.UserID,
		"file_path", payload.FilePath,
		"language", payload.Language,
	)

	// Verificar que el servicio esté disponible
	if h.ocrAI == nil {
		h.logger.Warn("AI OCR not available, falling back to Classic OCR",
			"job_id", payload.JobID,
		)
		return h.HandleOCRClassic(ctx, task)
	}

	// Procesar con OpenAI Vision
	result, err := h.ocrAI.ExtractText(ctx, payload.FilePath, payload.Language)
	if err != nil {
		h.logger.Error("OCR AI processing failed",
			"job_id", payload.JobID,
			"error", err,
		)
		
		// Registrar métrica de error AI
		metrics.RecordWorkerError("ocr", "ai_error")
		
		// Fallback a Classic OCR si falla AI
		h.logger.Info("Falling back to Classic OCR", "job_id", payload.JobID)
		return h.HandleOCRClassic(ctx, task)
	}

	// Guardar resultado
	resultPath := fmt.Sprintf("%s_result.txt", payload.FilePath)
	if err := os.WriteFile(resultPath, []byte(result.Text), 0644); err != nil {
		return fmt.Errorf("failed to save result: %w", err)
	}

	duration := time.Since(start)
	h.logger.Info("OCR AI job completed",
		"job_id", payload.JobID,
		"duration", duration,
		"text_length", len(result.Text),
		"confidence", result.Confidence,
	)

	// Registrar métricas de éxito
	metrics.RecordWorkerJobProcessed("ocr", "success", duration.Seconds(), "ai")
	metrics.RecordJobCompleted(queue.TypeOCRAI, payload.Plan, duration.Seconds())
	metrics.RecordJobResultSize(queue.TypeOCRAI, int64(len(result.Text)))

	return nil
}
