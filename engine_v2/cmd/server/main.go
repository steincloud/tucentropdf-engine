package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tucentropdf/engine-v2/internal/analytics"
	"github.com/tucentropdf/engine-v2/internal/api"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// @title TuCentroPDF Engine V2 API
// @version 2.0
// @description Motor de procesamiento PDF + Office + OCR con IA + Analytics
// @termsOfService https://tucentropdf.com/terms

// @contact.name TuCentroPDF Support
// @contact.email soporte@tucentropdf.com
// @contact.url https://tucentropdf.com/support

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api/v1
// @schemes http https

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-ENGINE-SECRET

func main() {
	// Cargar configuración
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Inicializar logger estructurado
	logger := logger.New(cfg.Log.Level, cfg.Log.Format)
	defer logger.Sync()

	logger.Info("🚀 Iniciando TuCentroPDF Engine V2 - Fase 3 Completa + Analytics",
		"version", "2.0.0",
		"environment", cfg.Environment,
		"port", cfg.Port,
		"ai_enabled", cfg.AI.Enabled,
		"ai_model", cfg.AI.Model,
		"redis_enabled", cfg.Redis.Enabled,
	)

	// Conectar a PostgreSQL (para analytics)
	db, err := connectDatabase(cfg, logger)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		// Continuar sin base de datos (analytics será deshabilitado)
		db = nil
	}

	// Ejecutar migraciones de analytics si DB está disponible
	if db != nil {
		if err := analytics.RunMigrations(db, logger); err != nil {
			logger.Error("Failed to run analytics migrations", "error", err)
			// Continuar sin analytics
			db = nil
		}
	}

	// Validaciones de configuración crítica de Fase 3
	logger.Info("🧠 Configuración de IA y límites por plan")
	if cfg.AI.Enabled {
		if cfg.AI.APIKey == "" {
			logger.Warn("⚠️ OpenAI API Key no configurada - OCR con IA estará deshabilitado")
		} else {
			logger.Info("🤖 OCR con IA habilitado",
				"model", cfg.AI.Model,
				"free_pages", cfg.Limits.Free.AIOCRPagesPerDay,
				"premium_pages", cfg.Limits.Premium.AIOCRPagesPerDay,
				"pro_pages", cfg.Limits.Pro.AIOCRPagesPerDay,
			)
		}
	}

	// Mostrar límites por plan implementados en Fase 3
	logger.Info("📊 Límites por plan configurados",
		"free_max_file_mb", cfg.Limits.Free.MaxFileSizeMB,
		"free_ai_pages", cfg.Limits.Free.AIOCRPagesPerDay,
		"free_rate_limit", cfg.Limits.Free.RateLimit,
		"premium_max_file_mb", cfg.Limits.Premium.MaxFileSizeMB,
		"premium_ai_pages", cfg.Limits.Premium.AIOCRPagesPerDay,
		"premium_rate_limit", cfg.Limits.Premium.RateLimit,
		"pro_max_file_mb", cfg.Limits.Pro.MaxFileSizeMB,
		"pro_ai_pages", cfg.Limits.Pro.AIOCRPagesPerDay,
		"pro_rate_limit", cfg.Limits.Pro.RateLimit,
	)

	// Crear servidor API con analytics y mantenimiento
	server := api.NewServer(cfg, logger, db)

	// Configurar graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Iniciar servidor en goroutine
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Port)
		logger.Info("🌐 Servidor iniciado", 
			"address", addr,
			"analytics_enabled", db != nil)
		if err := server.Listen(addr); err != nil {
			logger.Error("Error starting server", "error", err)
		}
	}()

	// Esperar señal de shutdown
	<-c
	logger.Info("🛑 Iniciando shutdown graceful...")

	// Timeout para shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown del servidor
	if err := server.ShutdownWithContext(ctx); err != nil {
		logger.Error("Error durante shutdown", "error", err)
	} else {
		logger.Info("✅ Servidor detenido correctamente")
	}

	// Cerrar conexión a base de datos
	if db != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	logger.Info("🎯 TuCentroPDF Engine V2 finalizado")
}

// connectDatabase conecta a la base de datos PostgreSQL con connection pool optimizado
func connectDatabase(cfg *config.Config, logger *logger.Logger) (*gorm.DB, error) {
	// Construir DSN desde variables de entorno
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_SSLMODE"),
	)

	// Si no hay configuración de DB, usar valores por defecto para desarrollo
	if os.Getenv("DB_HOST") == "" {
		logger.Info("📊 No database configuration found, analytics will be disabled")
		return nil, nil
	}

	logger.Info("📊 Connecting to PostgreSQL...", "host", os.Getenv("DB_HOST"))

	// Conectar a PostgreSQL
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configurar pool de conexiones optimizado para producci\u00f3n
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configuraci\u00f3n optimizada del pool de conexiones
	sqlDB.SetMaxIdleConns(10)           // M\u00ednimo de conexiones idle (reduce overhead)
	sqlDB.SetMaxOpenConns(50)           // M\u00e1ximo de conexiones abiertas (previene exhaust)
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // Recicla conexiones cada 30 min (previene stale connections)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Cierra conexiones idle despu\u00e9s de 10 min

	logger.Info("Database connection pool configured",
		"max_idle_conns", 10,
		"max_open_conns", 50,
		"conn_max_lifetime", "30m",
		"conn_max_idle_time", "10m",
	)

	// Test de conexión
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("✅ Database connected successfully")
	return db, nil
}