package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Load(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
	}{
		{
			name: "valid_config",
			envVars: map[string]string{
				"ENVIRONMENT":          "test",
				"PORT":                 "3000",
				"HOST":                 "localhost",
				"CORS_ORIGINS":         "http://localhost:3000",
				"OPENAI_API_KEY":       "test-key",
				"OPENAI_MODEL":         "gpt-4o-mini",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
				"REDIS_PASSWORD":       "",
				"STORAGE_TEMP_DIR":     "/tmp/tucentropdf",
				"STORAGE_UPLOADS_DIR":  "/tmp/uploads",
				"OFFICE_LIBREOFFICE_PATH": "soffice",
				"OFFICE_TIMEOUT":       "30",
				"LOG_LEVEL":           "info",
				"LOG_FORMAT":          "json",
			},
			expectError: false,
		},
		{
			name: "missing_required_vars",
			envVars: map[string]string{
				"ENVIRONMENT": "test",
				// Falta OPENAI_API_KEY
			},
			expectError: true,
		},
		{
			name: "invalid_port",
			envVars: map[string]string{
				"ENVIRONMENT":    "test",
				"PORT":           "invalid",
				"OPENAI_API_KEY": "test-key",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Backup y limpiar variables de entorno
			originalEnv := make(map[string]string)
			for key := range tt.envVars {
				originalEnv[key] = os.Getenv(key)
				os.Unsetenv(key)
			}
			
			defer func() {
				// Restaurar variables originales
				for key, value := range originalEnv {
					if value != "" {
						os.Setenv(key, value)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Configurar variables de test
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg, err := Load()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
				
				// Verificar valores específicos
				assert.Equal(t, "test", cfg.Environment)
				if tt.envVars["PORT"] != "" && tt.envVars["PORT"] != "invalid" {
					assert.Equal(t, 3000, cfg.Server.Port)
				}
				if tt.envVars["OPENAI_API_KEY"] != "" {
					assert.Equal(t, "test-key", cfg.AI.OpenAI.APIKey)
				}
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid_config",
			config:      getValidTestConfig(),
			expectError: false,
		},
		{
			name: "missing_openai_key",
			config: &Config{
				Environment: "production",
				AI: AIConfig{
					OpenAI: OpenAIConfig{
						APIKey: "", // Vacío
						Model:  "gpt-4o-mini",
					},
				},
			},
			expectError: true,
			errorMsg:    "OpenAI API key",
		},
		{
			name: "invalid_environment",
			config: &Config{
				Environment: "invalid",
			},
			expectError: true,
			errorMsg:    "environment must be",
		},
		{
			name: "invalid_port",
			config: &Config{
				Environment: "test",
				Server: ServerConfig{
					Port: -1,
				},
			},
			expectError: true,
			errorMsg:    "port must be",
		},
		{
			name: "invalid_log_level",
			config: &Config{
				Environment: "test",
				Server: ServerConfig{
					Port: 3000,
				},
				Logging: LoggingConfig{
					Level: "invalid",
				},
			},
			expectError: true,
			errorMsg:    "log level must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_IsDevelopment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		expected    bool
	}{
		{
			name:        "development_env",
			environment: "development",
			expected:    true,
		},
		{
			name:        "dev_env",
			environment: "dev",
			expected:    true,
		},
		{
			name:        "test_env",
			environment: "test",
			expected:    false,
		},
		{
			name:        "production_env",
			environment: "production",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Environment: tt.environment}
			assert.Equal(t, tt.expected, cfg.IsDevelopment())
		})
	}
}

func TestConfig_IsProduction(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		expected    bool
	}{
		{
			name:        "production_env",
			environment: "production",
			expected:    true,
		},
		{
			name:        "prod_env",
			environment: "prod",
			expected:    true,
		},
		{
			name:        "development_env",
			environment: "development",
			expected:    false,
		},
		{
			name:        "test_env",
			environment: "test",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Environment: tt.environment}
			assert.Equal(t, tt.expected, cfg.IsProduction())
		})
	}
}

func TestPlanLimitsConfig_GetPlanConfig(t *testing.T) {
	cfg := getValidTestConfig()

	tests := []struct {
		name     string
		plan     string
		expected config.PlanConfig
	}{
		{
			name: "free_plan",
			plan: "free",
			expected: config.PlanConfig{
				MaxFileSizeMB:  5,
				MaxFilesPerDay: 10,
				MaxAIOCRPages:  0,
			},
		},
		{
			name: "premium_plan",
			plan: "premium",
			expected: config.PlanConfig{
				MaxFileSizeMB:  25,
				MaxFilesPerDay: 100,
				MaxAIOCRPages:  3,
			},
		},
		{
			name: "pro_plan",
			plan: "pro",
			expected: config.PlanConfig{
				MaxFileSizeMB:  100,
				MaxFilesPerDay: 1000,
				MaxAIOCRPages:  20,
			},
		},
		{
			name: "unknown_plan_defaults_to_free",
			plan: "unknown",
			expected: config.PlanConfig{
				MaxFileSizeMB:  5,
				MaxFilesPerDay: 10,
				MaxAIOCRPages:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planCfg := cfg.GetPlanConfig(tt.plan)
			assert.Equal(t, tt.expected, planCfg)
		})
	}
}

func TestConfig_GetRedisAddr(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "localhost_redis",
			host:     "localhost",
			port:     6379,
			expected: "localhost:6379",
		},
		{
			name:     "remote_redis",
			host:     "redis.example.com",
			port:     6380,
			expected: "redis.example.com:6380",
		},
		{
			name:     "ip_address",
			host:     "192.168.1.100",
			port:     6379,
			expected: "192.168.1.100:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Redis: RedisConfig{
					Host: tt.host,
					Port: tt.port,
				},
			}
			assert.Equal(t, tt.expected, cfg.GetRedisAddr())
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	// Test que verifica que se asignan valores por defecto correctos
	cfg := &Config{}
	err := cfg.setDefaults()
	
	require.NoError(t, err)
	
	// Verificar defaults importantes
	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
	assert.Equal(t, "stdout", cfg.Logging.Output)
	
	// Verificar defaults de Redis
	assert.Equal(t, "localhost", cfg.Redis.Host)
	assert.Equal(t, 6379, cfg.Redis.Port)
	assert.Equal(t, 0, cfg.Redis.DB)
	
	// Verificar defaults de Storage
	assert.NotEmpty(t, cfg.Storage.TempDir)
	assert.NotEmpty(t, cfg.Storage.UploadsDir)
	
	// Verificar defaults de Office
	assert.Equal(t, 30, cfg.Office.Timeout)
	
	// Verificar defaults de Plan Limits
	assert.Equal(t, 5, cfg.PlanLimits.Free.MaxFileSizeMB)
	assert.Equal(t, 25, cfg.PlanLimits.Premium.MaxFileSizeMB)
	assert.Equal(t, 100, cfg.PlanLimits.Pro.MaxFileSizeMB)
}

func TestConfig_EnvVarParsing(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		envValue  string
		checkFunc func(*Config) bool
	}{
		{
			name:     "cors_origins_comma_separated",
			envVar:   "CORS_ORIGINS",
			envValue: "http://localhost:3000,https://example.com,https://app.tucentropdf.com",
			checkFunc: func(cfg *Config) bool {
				return len(cfg.Server.CORS.Origins) == 3 &&
					cfg.Server.CORS.Origins[0] == "http://localhost:3000" &&
					cfg.Server.CORS.Origins[1] == "https://example.com" &&
					cfg.Server.CORS.Origins[2] == "https://app.tucentropdf.com"
			},
		},
		{
			name:     "redis_port_parsing",
			envVar:   "REDIS_PORT",
			envValue: "6380",
			checkFunc: func(cfg *Config) bool {
				return cfg.Redis.Port == 6380
			},
		},
		{
			name:     "office_timeout_parsing",
			envVar:   "OFFICE_TIMEOUT",
			envValue: "45",
			checkFunc: func(cfg *Config) bool {
				return cfg.Office.Timeout == 45
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Backup variable original
			original := os.Getenv(tt.envVar)
			defer func() {
				if original != "" {
					os.Setenv(tt.envVar, original)
				} else {
					os.Unsetenv(tt.envVar)
				}
			}()

			// Configurar variable de test
			os.Setenv(tt.envVar, tt.envValue)
			os.Setenv("OPENAI_API_KEY", "test-key") // Requerida

			cfg, err := Load()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			assert.True(t, tt.checkFunc(cfg), "La función de verificación falló para %s=%s", tt.envVar, tt.envValue)
		})
	}
}

// Helper function para crear configuración válida de test
func getValidTestConfig() *Config {
	return &Config{
		Environment: "test",
		Server: ServerConfig{
			Port: 3000,
			Host: "localhost",
			CORS: CORSConfig{
				Origins: []string{"http://localhost:3000"},
			},
		},
		AI: AIConfig{
			OpenAI: OpenAIConfig{
				APIKey: "test-key",
				Model:  "gpt-4o-mini",
			},
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
		},
		Storage: StorageConfig{
			TempDir:    "/tmp/tucentropdf",
			UploadsDir: "/tmp/uploads",
		},
		Office: OfficeConfig{
			LibreOfficePath: "soffice",
			Timeout:         30,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		PlanLimits: PlanLimitsConfig{
			Free: PlanConfig{
				MaxFileSizeMB:  5,
				MaxFilesPerDay: 10,
				MaxAIOCRPages:  0,
			},
			Premium: PlanConfig{
				MaxFileSizeMB:  25,
				MaxFilesPerDay: 100,
				MaxAIOCRPages:  3,
			},
			Pro: PlanConfig{
				MaxFileSizeMB:  100,
				MaxFilesPerDay: 1000,
				MaxAIOCRPages:  20,
			},
		},
	}
}

// Tests de concurrencia para validar thread safety
func TestConfig_ConcurrentAccess(t *testing.T) {
	cfg := getValidTestConfig()
	
	// Test concurrente para GetPlanConfig
	t.Run("concurrent_plan_config_access", func(t *testing.T) {
		done := make(chan bool, 100)
		
		for i := 0; i < 100; i++ {
			go func(plan string) {
				defer func() { done <- true }()
				planCfg := cfg.GetPlanConfig(plan)
				assert.NotNil(t, planCfg)
			}("premium")
		}
		
		// Esperar a que terminen todas las goroutines
		for i := 0; i < 100; i++ {
			<-done
		}
	})
}

// Benchmark para operaciones de configuración
func BenchmarkConfig_GetPlanConfig(b *testing.B) {
	cfg := getValidTestConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.GetPlanConfig("premium")
	}
}

func BenchmarkConfig_Validate(b *testing.B) {
	cfg := getValidTestConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.Validate()
	}
}

func BenchmarkConfig_IsDevelopment(b *testing.B) {
	cfg := getValidTestConfig()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.IsDevelopment()
	}
}