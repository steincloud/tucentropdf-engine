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
	"github.com/tucentropdf/engine-v2/internal/office"
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

	logger.Info("📄 Starting Office Worker",
		"version", "2.0.0",
		"libreoffice_path", cfg.Office.LibreOfficePath,
	)

	// Inicializar servicios
	storageService := storage.NewService(cfg, logger)
	
	// Crear servicio Office
	officeService, err := office.NewService(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize LibreOffice service", "error", err)
		log.Fatalf("Office service initialization failed: %v", err)
	}

	// Configurar cola
	queueConfig := queue.LoadConfig(cfg)
	server := queue.NewOfficeServer(queueConfig)

	// Crear handler processor
	handler := &OfficeHandler{
		logger:         logger,
		config:         cfg,
		storageService: storageService,
		officeService:  officeService,
	}

	// Registrar task handlers
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeOfficeToPDF, handler.HandleOfficeToPDF)

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	// Iniciar servidor en goroutine
	// Registrar métricas de worker
	metrics.SetWorkerHealth("office", "default", true)
	metrics.SetWorkerConcurrency("office", queueConfig.OfficeConcurrency)

	go func() {
		logger.Info("🚀 Office Worker started", "concurrency", queueConfig.OfficeConcurrency)
		if err := server.Run(mux); err != nil {
			logger.Error("Office Worker failed", "error", err)
			metrics.SetWorkerHealth("office", "default", false)
			metrics.RecordWorkerRestart("office", "error")
		}
	}()

	// Esperar señal de shutdown
	<-done
	logger.Info("🛑 Shutting down Office Worker...")

	// Shutdown graceful (30 segundos)
	server.Shutdown()
	logger.Info("✅ Office Worker stopped")
}

// OfficeHandler maneja procesamiento de jobs de Office
type OfficeHandler struct {
	logger         *logger.Logger
	config         *config.Config
	storageService storage.Service
	officeService  office.Service
}

// HandleOfficeToPDF procesa conversión de Office a PDF
func (h *OfficeHandler) HandleOfficeToPDF(ctx context.Context, task *asynq.Task) error {
	start := time.Now()

	// Parsear payload
	var payload queue.OfficeJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		h.logger.Error("Failed to unmarshal Office payload", "error", err)
		return fmt.Errorf("invalid payload: %w", err)
	}

	h.logger.Info("Processing Office to PDF job",
		"job_id", payload.JobID,
		"user_id", payload.UserID,
		"file_path", payload.FilePath,
		"format", payload.Format,
	)

	// Verificar que el archivo existe
	if _, err := os.Stat(payload.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", payload.FilePath)
	}

	// Crear directorio de salida si no existe
	outputDir := h.storageService.(*storage.LocalStorage).GetTempDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Convertir a PDF
	pdfPath, err := h.officeService.ConvertToPDF(ctx, payload.FilePath, outputDir)
	if err != nil {
		h.logger.Error("Office conversion failed",
			"job_id", payload.JobID,
			"error", err,
		)
		// Registrar métrica de error
		metrics.RecordWorkerJobProcessed("office", "failed", time.Since(start).Seconds(), "to_pdf")
		metrics.RecordWorkerError("office", "conversion_error")
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Verificar que el PDF se creó correctamente
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return fmt.Errorf("PDF output not created")
	}

	// Obtener tamaño del archivo resultante
	fileInfo, err := os.Stat(pdfPath)
	if err != nil {
		h.logger.Warn("Failed to get PDF file info", "error", err)
	}

	duration := time.Since(start)
	h.logger.Info("Office to PDF job completed",
		"job_id", payload.JobID,
		"duration", duration,
		"input_path", payload.FilePath,
		"output_path", pdfPath,
		"output_size_mb", float64(fileInfo.Size())/(1024*1024),
	)

	// Registrar métricas de éxito
	metrics.RecordWorkerJobProcessed("office", "success", duration.Seconds(), "to_pdf")
	metrics.RecordJobCompleted(queue.TypeOfficeToPDF, payload.Plan, duration.Seconds())
	metrics.RecordJobResultSize(queue.TypeOfficeToPDF, fileInfo.Size())

	// Opcional: Guardar metadata del resultado en Redis
	// result := queue.JobResult{
	// 	JobID:      payload.JobID,
	// 	Status:     queue.StatusCompleted,
	// 	ResultPath: pdfPath,
	// 	Duration:   duration,
	// 	CompletedAt: time.Now(),
	// }
	// TODO: Store result in Redis

	return nil
}
