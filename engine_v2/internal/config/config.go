package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config estructura principal de configuración
type Config struct {
	Environment string `json:"environment"`
	Port        int    `json:"port"`
	EngineSecret string `json:"-"` // No exponer en JSON

	// Logging
	Log LogConfig `json:"log"`

	// AI/OCR
	AI  AIConfig  `json:"ai"`
	OCR OCRConfig `json:"ocr"`

	// Office
	Office OfficeConfig `json:"office"`

	// Límites por plan
	Limits LimitsConfig `json:"limits"`

	// Compatibilidad legacy: algunos tests y código esperan el campo `PlanLimits`.
	// Mantener para retrocompatibilidad; preferir `Limits` en nuevo código.
	PlanLimits PlanLimitsConfig `json:"plan_limits"`

	// Storage
	Storage StorageConfig `json:"storage"`

	// Security
	Security SecurityConfig `json:"security"`

	// Redis
	Redis RedisConfig `json:"redis"`

	// Alerts
	Alerts AlertsConfig `json:"alerts"`

	// Captcha
	Captcha CaptchaConfig `json:"captcha"`
}

type CaptchaConfig struct {
	Enabled   bool   `json:"enabled"`
	SiteKey   string `json:"site_key"`
	SecretKey string `json:"-"` // No exponer
	Version   string `json:"version"` // v2 o v3
}

type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
	Output string `json:"output"`
}

type AIConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"-"` // No exponer
	Enabled  bool   `json:"enabled"`
}

type OCRConfig struct {
	Provider       string   `json:"provider"`
	Languages      []string `json:"languages"`
	PaddleEnabled  bool     `json:"paddle_enabled"`
	TesseractPath  string   `json:"tesseract_path"`
}

type OfficeConfig struct {
	Enabled        bool   `json:"enabled"`
	Provider       string `json:"provider"`
	LibreOfficePath string `json:"libreoffice_path"`
	GotenbergURL   string `json:"gotenberg_url"`
}

type LimitsConfig struct {
	Free      PlanLimits `json:"free"`
	Premium   PlanLimits `json:"premium"`
	Pro       PlanLimits `json:"pro"`
	Corporate PlanLimits `json:"corporate"`
}

// Compatibilidad: tipos legacy usados en tests antiguos y en código legado.
// Estas definiciones no cambian la lógica principal pero permiten que los
// tests y módulos que aún esperan estos nombres compilen sin cambios.
type PlanConfig struct {
	MaxFileSizeMB  int `json:"max_file_size_mb"`
	MaxFilesPerDay int `json:"max_files_per_day"`
	MaxAIOCRPages  int `json:"max_ai_ocr_pages"`
}

type PlanLimitsConfig struct {
	Free    PlanConfig `json:"free"`
	Premium PlanConfig `json:"premium"`
	Pro     PlanConfig `json:"pro"`
}

// PlanConfig se mantiene para retrocompatibilidad pero ahora usa PlanLimits internamente
// TODO: Deprecar en futuras versiones en favor de PlanLimits directamente

type StorageConfig struct {
	TempDir         string `json:"temp_dir"`
	CleanupInterval int    `json:"cleanup_interval"`
	MaxTempAge      int    `json:"max_temp_age"`
	MaxStorageSize  string `json:"max_storage_size"`
}

type SecurityConfig struct {
	MaxRequestSize       string   `json:"max_request_size"`
	AllowedOrigins       []string `json:"allowed_origins"`
	EnableCORS           bool     `json:"enable_cors"`
	EnableRateLimiting   bool     `json:"enable_rate_limiting"`
}

type RedisConfig struct {
	Enabled  bool   `json:"enabled"`
	URL      string `json:"url"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Password string `json:"-"` // No exponer
	DB       int    `json:"db"`
}

type AlertsConfig struct {
	EmailEnabled       bool   `json:"email_enabled"`
	TelegramEnabled    bool   `json:"telegram_enabled"`
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUsername      string `json:"smtp_username"`
	SMTPPassword      string `json:"-"` // No exponer
	EmailFrom         string `json:"email_from"`
	EmailTo           []string `json:"email_to"`
	TelegramBotToken  string `json:"-"` // No exponer
	TelegramChatID    string `json:"telegram_chat_id"`
}

// Load carga la configuración desde variables de entorno
func Load() (*Config, error) {
	// Cargar .env si existe
	if err := godotenv.Load(".env"); err != nil {
		// No es error crítico si no existe .env
	}

	cfg := &Config{
		Environment:  getEnv("ENVIRONMENT", "development"),
		Port:         getEnvInt("PORT", 8080),
		EngineSecret: getEnv("ENGINE_SECRET", ""),

		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
			Output: getEnv("LOG_OUTPUT", "stdout"),
		},

		AI: AIConfig{
			Provider: getEnv("AI_PROVIDER", "openai"),
			Model:    getEnv("AI_MODEL", "gpt-4o-mini"),
			APIKey:   getEnv("OPENAI_API_KEY", ""),
			Enabled:  getEnvBool("AI_OCR_ENABLED", true),
		},

		OCR: OCRConfig{
			Provider:      getEnv("OCR_PROVIDER", "tesseract"),
			Languages:     strings.Split(getEnv("OCR_LANGUAGES", "eng,spa"), ","),
			PaddleEnabled: getEnvBool("PADDLE_OCR_ENABLED", true),
			TesseractPath: getEnv("TESSERACT_PATH", "/usr/bin/tesseract"),
		},

		Office: OfficeConfig{
			Enabled:         getEnvBool("OFFICE_ENABLED", true),
			Provider:        getEnv("OFFICE_PROVIDER", "libreoffice"),
			LibreOfficePath: getEnv("LIBREOFFICE_PATH", "/usr/bin/libreoffice"),
			GotenbergURL:    getEnv("GOTENBERG_URL", "http://localhost:3000"),
		},

		// Configuración de planes (se carga desde plans.go)
		Limits: LimitsConfig{
			Free:      getEnvLimitsOrDefault("FREE", GetDefaultPlanConfiguration().Plans[PlanFree]),
			Premium:   getEnvLimitsOrDefault("PREMIUM", GetDefaultPlanConfiguration().Plans[PlanPremium]),
			Pro:       getEnvLimitsOrDefault("PRO", GetDefaultPlanConfiguration().Plans[PlanPro]),
			Corporate: getEnvLimitsOrDefault("CORPORATE", GetDefaultPlanConfiguration().Plans[PlanCorporate]),
		},

		Storage: StorageConfig{
			TempDir:         getEnv("TEMP_DIR", "/tmp/tucentropdf-v2"),
			CleanupInterval: getEnvInt("CLEANUP_INTERVAL", 300),
			MaxTempAge:      getEnvInt("MAX_TEMP_AGE", 3600),
			MaxStorageSize:  getEnv("MAX_STORAGE_SIZE", "10GB"),
		},

		Security: SecurityConfig{
			MaxRequestSize:     getEnv("MAX_REQUEST_SIZE", "250MB"),
			AllowedOrigins:     strings.Split(getEnv("ALLOWED_ORIGINS", "*"), ","),
			EnableCORS:         getEnvBool("ENABLE_CORS", true),
			EnableRateLimiting: getEnvBool("ENABLE_RATE_LIMITING", true),
		},

		       Redis: RedisConfig{
			       Enabled:  getEnvBool("REDIS_ENABLED", false),
			       URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			       Host:     getEnv("REDIS_HOST", "localhost"),
			       Port:     getEnv("REDIS_PORT", "6379"),
			       Password: getEnv("REDIS_PASSWORD", ""),
			       DB:       getEnvInt("REDIS_DB", 0),
		       },

		Alerts: AlertsConfig{
			EmailEnabled:      getEnvBool("ALERTS_EMAIL_ENABLED", false),
			TelegramEnabled:   getEnvBool("ALERTS_TELEGRAM_ENABLED", false),
			SMTPHost:         getEnv("SMTP_HOST", ""),
			SMTPPort:         getEnvInt("SMTP_PORT", 587),
			SMTPUsername:     getEnv("SMTP_USERNAME", ""),
			SMTPPassword:     getEnv("SMTP_PASSWORD", ""),
			EmailFrom:        getEnv("EMAIL_FROM", ""),
			EmailTo:          strings.Split(getEnv("EMAIL_TO", ""), ","),
			TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
			TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		},

		Captcha: CaptchaConfig{
			Enabled:   getEnvBool("CAPTCHA_ENABLED", false),
			SiteKey:   getEnv("CAPTCHA_SITE_KEY", ""),
			SecretKey: getEnv("CAPTCHA_SECRET_KEY", ""),
			Version:   getEnv("CAPTCHA_VERSION", "v3"),
		},
	}

	// Validar configuración crítica
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate valida la configuración
func (c *Config) Validate() error {
	if c.EngineSecret == "" {
		return fmt.Errorf("ENGINE_SECRET is required")
	}

	if len(c.EngineSecret) < 32 {
		return fmt.Errorf("ENGINE_SECRET must be at least 32 characters")
	}

	if c.AI.Enabled && c.AI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required when AI is enabled")
	}

	return nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvLimitsOrDefault obtiene límites de variables de entorno o usa defaults
func getEnvLimitsOrDefault(planPrefix string, defaultLimits PlanLimits) PlanLimits {
	limits := defaultLimits
	
	// Permitir override por variables de entorno
	if val := getEnvInt(fmt.Sprintf("PLAN_%s_MAX_FILE_SIZE_MB", planPrefix), 0); val > 0 {
		limits.MaxFileSizeMB = val
	}
	if val := getEnvInt(fmt.Sprintf("PLAN_%s_MAX_PAGES", planPrefix), 0); val > 0 {
		limits.MaxPages = val
	}
	if val := getEnvInt(fmt.Sprintf("PLAN_%s_DAILY_OPERATIONS", planPrefix), 0); val > 0 {
		limits.DailyOperations = val
	}
	if val := getEnvInt(fmt.Sprintf("PLAN_%s_MONTHLY_OPERATIONS", planPrefix), 0); val > 0 {
		limits.MonthlyOperations = val
	}
	if val := getEnvInt(fmt.Sprintf("PLAN_%s_RATE_LIMIT", planPrefix), 0); val > 0 {
		limits.RateLimit = val
	}
	
	return limits
}