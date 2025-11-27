package api

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/api/middleware"
	"github.com/tucentropdf/engine-v2/internal/api/routes"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Server encapsula el servidor Fiber
type Server struct {
	app    *fiber.App
	config *config.Config
	logger *logger.Logger
	redis  *redis.Client
	db     *gorm.DB
}

// NewServer crea una nueva instancia del servidor
func NewServer(cfg *config.Config, log *logger.Logger, db *gorm.DB) *Server {
	// Configurar Redis si está habilitado
	var redisClient *redis.Client
	if cfg.Redis.Enabled && cfg.Redis.URL != "" {
		redisClient = setupRedis(cfg, log)
		log.Info("🟥 Redis configurado", "url", cfg.Redis.URL)
	} else {
		log.Warn("⚠️ Redis deshabilitado - usando fallbacks en memoria")
	}

	// Configuración de Fiber
	fiberConfig := fiber.Config{
		AppName:               "TuCentroPDF Engine V2",
		ServerHeader:          "TuCentroPDF/2.0",
		DisableStartupMessage: true,
		BodyLimit:             250 * 1024 * 1024, // 250MB
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           120 * time.Second,
		ErrorHandler:          middleware.ErrorHandler(log),
	}

	app := fiber.New(fiberConfig)

	// Middleware global
	setupMiddleware(app, cfg, log)

	// Configurar rutas con Redis y DB
	routes.Setup(app, cfg, log, redisClient, db)

	return &Server{
		app:    app,
		config: cfg,
		logger: log,
		redis:  redisClient,
		db:     db,
	}
}

// setupMiddleware configura middleware global
func setupMiddleware(app *fiber.App, cfg *config.Config, log *logger.Logger) {
	// Recovery middleware
	app.Use(recover.New())

	// Security headers
	app.Use(helmet.New(helmet.Config{
		XSSProtection:             "1; mode=block",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		ReferrerPolicy:            "no-referrer",
		CrossOriginEmbedderPolicy: "require-corp",
	}))

	// CORS
	if cfg.Security.EnableCORS {
		app.Use(cors.New(cors.Config{
			AllowOrigins:     string(cfg.Security.AllowedOrigins[0]),
			AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
			AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-ENGINE-SECRET",
			ExposeHeaders:    "Content-Length",
			AllowCredentials: false,
			MaxAge:           12 * 60 * 60, // 12 hours
		}))
	}

	// Request logging
	app.Use(middleware.RequestLogger(log))

	// Rate limiting global
	if cfg.Security.EnableRateLimiting {
		app.Use(limiter.New(limiter.Config{
			Max:        100, // requests
			Expiration: 1 * time.Hour,
			KeyGenerator: func(c *fiber.Ctx) string {
				return c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				log.Warn("Rate limit exceeded", "ip", c.IP())
				return c.Status(429).JSON(fiber.Map{
					"error":   "Too Many Requests",
					"message": "Rate limit exceeded. Try again later.",
					"code":    "RATE_LIMIT_EXCEEDED",
				})
			},
		}))
	}

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":    "ok",
			"service":   "tucentropdf-engine-v2",
			"version":   "2.0.0",
			"timestamp": time.Now().UTC(),
		})
	})

	// Swagger documentation
	if cfg.Environment == "development" {
		app.Get("/docs/*", swagger.HandlerDefault)
	}
}

// setupRedis configurar cliente Redis
func setupRedis(cfg *config.Config, log *logger.Logger) *redis.Client {
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		log.Error("Error parseando URL de Redis", "url", cfg.Redis.URL, "error", err.Error())
		return nil
	}

	if cfg.Redis.Password != "" {
		opt.Password = cfg.Redis.Password
	}

	client := redis.NewClient(opt)

	// Probar conexión
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Ping(ctx).Result()
	if err != nil {
		log.Error("Error conectando a Redis", "error", err.Error())
		return nil
	}

	log.Info("✅ Redis conectado exitosamente")
	return client
}

// Listen inicia el servidor
func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

// Start inicia el servidor en el puerto configurado
func (s *Server) Start() error {
	s.logger.Info("🚀 Iniciando TuCentroPDF Engine V2",
		"port", s.config.Port,
		"environment", s.config.Environment,
		"ai_enabled", s.config.AI.Enabled,
		"redis_enabled", s.config.Redis.Enabled,
	)

	// Validar configuración crítica
	if s.config.AI.Enabled && s.config.AI.APIKey == "" {
		s.logger.Warn("⚠️ OpenAI API Key no configurada - OCR con IA deshabilitado")
	}

	return s.app.Listen(fmt.Sprintf(":%d", s.config.Port))
}

// Shutdown detiene el servidor gracefully
func (s *Server) Shutdown() error {
	s.logger.Info("🛑 Deteniendo servidor...")
	
	// Cerrar conexión Redis si existe
	if s.redis != nil {
		if err := s.redis.Close(); err != nil {
			s.logger.Error("Error cerrando conexión Redis", "error", err.Error())
		}
	}

	return s.app.Shutdown()
}

// ShutdownWithContext detiene el servidor con context
func (s *Server) ShutdownWithContext(ctx interface{}) error {
	return s.app.Shutdown()
}