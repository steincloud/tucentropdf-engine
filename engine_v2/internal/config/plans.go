package config

import (
	"time"
)

// Plan representa un plan de suscripción
type Plan string

const (
	PlanFree      Plan = "free"
	PlanPremium   Plan = "premium" 
	PlanPro       Plan = "pro"
	PlanCorporate Plan = "corporate"
)

// String implementa Stringer interface
func (p Plan) String() string {
	return string(p)
}

// IsValid verifica si el plan es válido
func (p Plan) IsValid() bool {
	switch p {
	case PlanFree, PlanPremium, PlanPro, PlanCorporate:
		return true
	default:
		return false
	}
}

// BillingCycle representa el ciclo de facturación
type BillingCycle string

const (
	BillingMonthly BillingCycle = "monthly"
	BillingYearly  BillingCycle = "yearly"
)

// String implementa Stringer interface
func (b BillingCycle) String() string {
	return string(b)
}

// PlanLimits define los límites específicos de cada plan (VISIBLES AL USUARIO)
type PlanLimits struct {
	// Límites de archivos visibles
	MaxFileSizeMB        int `json:"max_file_size_mb"`        // Límite visible por archivo
	MaxPages             int `json:"max_pages"`               // Páginas máximas por archivo
	MaxFilesPerDay       int `json:"max_files_per_day"`       // Archivos por día
	MaxFilesPerMonth     int `json:"max_files_per_month"`     // Archivos por mes
	MaxConcurrentFiles   int `json:"max_concurrent_files"`    // Archivos simultáneos
	
	// Límites de operaciones visibles
	DailyOperations   int `json:"daily_operations"`   // Operaciones por día
	MonthlyOperations int `json:"monthly_operations"` // Operaciones por mes
	
	// Límites de OCR visibles
	OCRPagesPerDay        int `json:"ocr_pages_per_day"`         // OCR básico por día
	OCRPagesPerMonth      int `json:"ocr_pages_per_month"`       // OCR básico por mes
	AIOCRPagesPerDay      int `json:"ai_ocr_pages_per_day"`      // OCR con IA por día
	AIOCRPagesPerMonth    int `json:"ai_ocr_pages_per_month"`    // OCR con IA por mes
	
	// Límites de Office visibles
	OfficePagesPerDay     int `json:"office_pages_per_day"`      // Office por día
	OfficePagesPerMonth   int `json:"office_pages_per_month"`    // Office por mes
	OfficeHasWatermark    bool `json:"office_has_watermark"`     // Marca de agua en Office
	
	// Límites de transferencia visibles
	MaxBytesPerDay   int64 `json:"max_bytes_per_day"`   // Bytes por día
	MaxBytesPerMonth int64 `json:"max_bytes_per_month"` // Bytes por mes
	
	// Configuración de procesamiento visible
	RateLimit         int           `json:"rate_limit"`          // Requests por minuto
	Priority          int           `json:"priority"`            // Prioridad (1=baja, 5=media, 10=alta)
	ProcessingTimeout time.Duration `json:"processing_timeout"` // Timeout visible
	SpeedLevel        string        `json:"speed_level"`         // "low", "medium", "high", "turbo"
	
	// Features habilitadas visibles
	EnableAIOCR         bool   `json:"enable_ai_ocr"`         // OCR con IA habilitado
	EnablePriority      bool   `json:"enable_priority"`       // Procesamiento prioritario
	EnableAnalytics     bool   `json:"enable_analytics"`      // Analytics avanzados
	EnableTeamAccess    bool   `json:"enable_team_access"`    // Acceso de equipo
	EnableAPI           bool   `json:"enable_api"`            // API y webhooks
	EnableAdvancedFeats bool   `json:"enable_advanced_feats"` // Features avanzadas
	HasWatermark        bool   `json:"has_watermark"`         // Marca de agua general
	HasAds              bool   `json:"has_ads"`               // Publicidad visible
	SupportLevel        string `json:"support_level"`         // "auto", "email", "priority", "dedicated"
	MaxTeamUsers        int    `json:"max_team_users"`        // Usuarios en equipo
	
	// Límites internos invisibles (NO se muestran al usuario)
	InternalLimits *InternalLimits `json:"-"`
}

// InternalLimits límites invisibles de protección del servidor
type InternalLimits struct {
	// Límites absolutos de protección (NUNCA visibles al usuario)
	AbsoluteMaxFileSize   int64         `json:"-"` // 350MB límite absoluto
	AbsoluteMaxQueueSize  int           `json:"-"` // 50 jobs simultáneos globales
	AbsoluteMaxCPUPercent int           `json:"-"` // 85% CPU máximo
	AbsoluteMaxRAMPercent int           `json:"-"` // 80% RAM máximo
	
	// Límites de contenedor (INVISIBLES)
	ContainerCPULimit    float64       `json:"-"` // CPU por contenedor
	ContainerRAMLimit    int64         `json:"-"` // RAM por contenedor
	ContainerCPUReserve  float64       `json:"-"` // CPU reservada
	ContainerRAMReserve  int64         `json:"-"` // RAM reservada
	
	// Timeouts estrictos (INVISIBLES)
	OCRTimeoutSeconds    int           `json:"-"` // 60s máximo OCR
	OfficeTimeoutSeconds int           `json:"-"` // 120s máximo Office
	PDFTimeoutSeconds    int           `json:"-"` // 45s máximo PDF
	
	// Control de cola (INVISIBLE)
	QueuePauseThreshold  int           `json:"-"` // Pausar nuevos jobs si cola > X
	HighLoadFileLimit    int64         `json:"-"` // Bajar límite de archivos si carga alta
	ProtectorModeSeconds int           `json:"-"` // Duración modo protector
	
	// Monitoreo y recuperación automática (INVISIBLE)
	HealthCheckInterval  time.Duration `json:"-"` // Intervalo de health checks
	AutoRestartEnabled   bool          `json:"-"` // Reinicio automático workers
	MaxWorkerHangTime    time.Duration `json:"-"` // Tiempo máximo antes de restart
}

// PlanPricing define los precios de cada plan
type PlanPricing struct {
	Monthly float64 `json:"monthly"`
	Yearly  float64 `json:"yearly"`
}

// PlanConfiguration configuración completa de planes
type PlanConfiguration struct {
	Plans map[Plan]PlanLimits `json:"plans"`
	Pricing map[Plan]PlanPricing `json:"pricing"`
}

// GetDefaultPlanConfiguration retorna la configuración por defecto de planes
func GetDefaultPlanConfiguration() *PlanConfiguration {
	return &PlanConfiguration{
		Plans: map[Plan]PlanLimits{
			PlanFree: {
				// === PLAN FREE ===
				// Límites visibles al usuario
				MaxFileSizeMB:       10,  // 10MB por archivo (visible)
				MaxPages:           20,   // 20 páginas por archivo
				MaxFilesPerDay:     10,   // 10 archivos por día
				MaxFilesPerMonth:   200,  // 200 archivos por mes
				MaxConcurrentFiles: 1,    // 1 archivo simultáneo
				
				DailyOperations:   10,   // 10 operaciones por día
				MonthlyOperations: 200,  // 200 operaciones por mes
				
				// OCR básico limitado
				OCRPagesPerDay:    2,    // OCR básico: 2 páginas/día
				OCRPagesPerMonth: 30,    // OCR básico: 30 páginas/mes
				AIOCRPagesPerDay: 0,     // ❌ Sin IA OCR
				AIOCRPagesPerMonth: 0,   // ❌ Sin IA OCR
				
				// Office con marca de agua si supera límites
				OfficePagesPerDay:   3,    // 3 páginas Office/día
				OfficePagesPerMonth: 60,   // 60 páginas Office/mes
				OfficeHasWatermark: true,  // ✅ Marca de agua en Office
				
				MaxBytesPerDay:   50 * 1024 * 1024,  // 50MB por día
				MaxBytesPerMonth: 1024 * 1024 * 1024, // 1GB por mes
				
				// Configuración visible
				RateLimit:         10,                // 10 req/min
				Priority:          1,                 // Prioridad baja
				ProcessingTimeout: 30 * time.Second, // 30s timeout visible
				SpeedLevel:        "low",             // Velocidad baja
				
				// Features visibles
				EnableAIOCR:         false,            // ❌ Sin IA OCR
				EnablePriority:      false,            // ❌ Sin prioridad
				EnableAnalytics:     false,            // ❌ Sin analytics
				EnableTeamAccess:    false,            // ❌ Sin equipo
				EnableAPI:           false,            // ❌ Sin API
				EnableAdvancedFeats: false,            // ❌ Sin features avanzadas
				HasWatermark:        true,             // ✅ Marca de agua
				HasAds:              true,             // ✅ Publicidad obligatoria
				SupportLevel:        "auto",           // Soporte automático
				MaxTeamUsers:        0,                // Sin equipo
				
				// Límites internos invisibles de protección
				InternalLimits: &InternalLimits{
					AbsoluteMaxFileSize:   350 * 1024 * 1024, // 350MB absoluto (invisible)
					AbsoluteMaxQueueSize:  50,                 // 50 jobs globales máximo
					AbsoluteMaxCPUPercent: 85,                 // 85% CPU máximo
					AbsoluteMaxRAMPercent: 80,                 // 80% RAM máximo
					
					ContainerCPULimit:   0.5,          // 0.5 CPU para engine
					ContainerRAMLimit:   300 << 20,    // 300MB RAM para engine
					ContainerCPUReserve: 0.1,          // 0.1 CPU reservado
					ContainerRAMReserve: 64 << 20,     // 64MB RAM reservado
					
					OCRTimeoutSeconds:    60,          // 60s máximo OCR
					OfficeTimeoutSeconds: 120,         // 120s máximo Office
					PDFTimeoutSeconds:    45,          // 45s máximo PDF
					
					QueuePauseThreshold:  20,          // Pausar si cola > 20
					HighLoadFileLimit:    5 << 20,     // 5MB en carga alta
					ProtectorModeSeconds: 300,         // 5 min modo protector
					
					HealthCheckInterval: 30 * time.Second, // Health check cada 30s
					AutoRestartEnabled:  true,             // Auto restart habilitado
					MaxWorkerHangTime:   120 * time.Second, // 2 min antes restart
				},
			},
			PlanPremium: {
				// === PLAN PREMIUM ===
				// Límites visibles generosos
				MaxFileSizeMB:       50,   // 50MB por archivo (visible)
				MaxPages:           100,   // 100 páginas por archivo
				MaxFilesPerDay:     7,     // ~7 archivos por día (200/mes)
				MaxFilesPerMonth:   200,   // 200 archivos por mes
				MaxConcurrentFiles: 3,     // 3 archivos simultáneos
				
				DailyOperations:   100,   // 100 operaciones por día
				MonthlyOperations: 2000,  // 2000 operaciones por mes
				
				// OCR completo
				OCRPagesPerDay:    50,    // OCR básico: 50 páginas/día
				OCRPagesPerMonth: 1000,   // OCR básico: 1000 páginas/mes
				AIOCRPagesPerDay: 5,      // ✅ IA OCR limitado: 5/día
				AIOCRPagesPerMonth: 100,  // ✅ IA OCR limitado: 100/mes
				
				// Office completo sin marca de agua
				OfficePagesPerDay:   50,    // 50 páginas Office/día
				OfficePagesPerMonth: 1000,  // 1000 páginas Office/mes
				OfficeHasWatermark: false,  // ❌ Sin marca de agua
				
				MaxBytesPerDay:   500 * 1024 * 1024,  // 500MB por día
				MaxBytesPerMonth: 10 * 1024 * 1024 * 1024, // 10GB por mes
				
				// Configuración visible mejorada
				RateLimit:         60,                // 60 req/min
				Priority:          5,                 // Prioridad media
				ProcessingTimeout: 60 * time.Second, // 60s timeout visible
				SpeedLevel:        "medium",          // Velocidad media
				
				// Features Premium
				EnableAIOCR:         true,             // ✅ IA OCR limitada
				EnablePriority:      true,             // ✅ Prioridad media
				EnableAnalytics:     true,             // ✅ Analytics básicos
				EnableTeamAccess:    false,            // ❌ Sin equipo aún
				EnableAPI:           false,            // ❌ Sin API aún
				EnableAdvancedFeats: false,            // ❌ Sin features avanzadas
				HasWatermark:        false,            // ❌ Sin marca de agua
				HasAds:              false,            // ❌ Sin publicidad
				SupportLevel:        "email",          // Soporte por email
				MaxTeamUsers:        0,                // Sin equipo aún
				
				// Límites internos invisibles mejorados
				InternalLimits: &InternalLimits{
					AbsoluteMaxFileSize:   350 * 1024 * 1024, // 350MB absoluto (invisible)
					AbsoluteMaxQueueSize:  50,                 // 50 jobs globales máximo
					AbsoluteMaxCPUPercent: 85,                 // 85% CPU máximo
					AbsoluteMaxRAMPercent: 80,                 // 80% RAM máximo
					
					ContainerCPULimit:   1.0,          // 1.0 CPU para engine
					ContainerRAMLimit:   512 << 20,    // 512MB RAM para engine
					ContainerCPUReserve: 0.2,          // 0.2 CPU reservado
					ContainerRAMReserve: 128 << 20,    // 128MB RAM reservado
					
					OCRTimeoutSeconds:    60,          // 60s máximo OCR
					OfficeTimeoutSeconds: 120,         // 120s máximo Office
					PDFTimeoutSeconds:    45,          // 45s máximo PDF
					
					QueuePauseThreshold:  30,          // Pausar si cola > 30
					HighLoadFileLimit:    25 << 20,    // 25MB en carga alta
					ProtectorModeSeconds: 300,         // 5 min modo protector
					
					HealthCheckInterval: 30 * time.Second, // Health check cada 30s
					AutoRestartEnabled:  true,             // Auto restart habilitado
					MaxWorkerHangTime:   120 * time.Second, // 2 min antes restart
				},
			},
			PlanPro: {
				// === PLAN PRO ===
				// Límites visibles altos
				MaxFileSizeMB:       200,  // 200MB por archivo (visible)
				MaxPages:           500,   // 500 páginas por archivo
				MaxFilesPerDay:     167,   // ~167 archivos por día (5000/mes)
				MaxFilesPerMonth:   5000,  // 5000 archivos por mes
				MaxConcurrentFiles: 10,    // 10 archivos simultáneos
				
				DailyOperations:   500,    // 500 operaciones por día
				MonthlyOperations: 10000,  // 10000 operaciones por mes
				
				// OCR avanzado + IA completo
				OCRPagesPerDay:    200,   // OCR básico: 200 páginas/día
				OCRPagesPerMonth: 5000,   // OCR básico: 5000 páginas/mes
				AIOCRPagesPerDay: 50,     // ✅ IA OCR avanzado: 50/día
				AIOCRPagesPerMonth: 1000, // ✅ IA OCR avanzado: 1000/mes
				
				// Office + Turbo
				OfficePagesPerDay:   200,   // 200 páginas Office/día
				OfficePagesPerMonth: 5000,  // 5000 páginas Office/mes
				OfficeHasWatermark: false,  // ❌ Sin marca de agua
				
				MaxBytesPerDay:   2 * 1024 * 1024 * 1024,   // 2GB por día
				MaxBytesPerMonth: 50 * 1024 * 1024 * 1024,  // 50GB por mes
				
				// Configuración visible alta performance
				RateLimit:         300,                // 300 req/min
				Priority:          10,                 // Prioridad alta
				ProcessingTimeout: 120 * time.Second, // 120s timeout visible
				SpeedLevel:        "high",             // Velocidad alta
				
				// Features Pro completas
				EnableAIOCR:         true,             // ✅ IA OCR completo
				EnablePriority:      true,             // ✅ Prioridad alta
				EnableAnalytics:     true,             // ✅ Analytics avanzados
				EnableTeamAccess:    true,             // ✅ Acceso Team: 5 usuarios
				EnableAPI:           true,             // ✅ API + webhooks
				EnableAdvancedFeats: true,             // ✅ Carpetas inteligentes + Jobs
				HasWatermark:        false,            // ❌ Sin marca de agua
				HasAds:              false,            // ❌ Sin publicidad
				SupportLevel:        "priority",       // Soporte prioritario (1h)
				MaxTeamUsers:        5,                // Equipo de 5 usuarios
				
				// Límites internos invisibles optimizados
				InternalLimits: &InternalLimits{
					AbsoluteMaxFileSize:   350 * 1024 * 1024, // 350MB absoluto (invisible)
					AbsoluteMaxQueueSize:  50,                 // 50 jobs globales máximo
					AbsoluteMaxCPUPercent: 85,                 // 85% CPU máximo
					AbsoluteMaxRAMPercent: 80,                 // 80% RAM máximo
					
					ContainerCPULimit:   2.0,          // 2.0 CPU para engine
					ContainerRAMLimit:   1024 << 20,   // 1GB RAM para engine
					ContainerCPUReserve: 0.5,          // 0.5 CPU reservado
					ContainerRAMReserve: 256 << 20,    // 256MB RAM reservado
					
					OCRTimeoutSeconds:    60,          // 60s máximo OCR
					OfficeTimeoutSeconds: 120,         // 120s máximo Office
					PDFTimeoutSeconds:    45,          // 45s máximo PDF
					
					QueuePauseThreshold:  40,          // Pausar si cola > 40
					HighLoadFileLimit:    100 << 20,   // 100MB en carga alta
					ProtectorModeSeconds: 300,         // 5 min modo protector
					
					HealthCheckInterval: 15 * time.Second, // Health check cada 15s
					AutoRestartEnabled:  true,             // Auto restart habilitado
					MaxWorkerHangTime:   60 * time.Second, // 1 min antes restart
				},
			},
			PlanCorporate: {
				// === PLAN CORPORATIVO ===
				// "Ilimitado" para el usuario, pero con límites internos invisibles
				MaxFileSizeMB:       500,  // 500MB "ilimitado" visible
				MaxPages:           2000,  // 2000 páginas "ilimitado"
				MaxFilesPerDay:     1000,  // "Ilimitado" diario
				MaxFilesPerMonth:   30000, // "Ilimitado" mensual
				MaxConcurrentFiles: 50,    // 50 archivos simultáneos
				
				DailyOperations:   2000,  // "Ilimitado" operaciones
				MonthlyOperations: 50000, // "Ilimitado" mensual
				
				// OCR + IA "ilimitado"
				OCRPagesPerDay:    1000,   // "Ilimitado" OCR
				OCRPagesPerMonth: 25000,   // "Ilimitado" OCR
				AIOCRPagesPerDay: 500,     // "Ilimitado" IA OCR
				AIOCRPagesPerMonth: 10000, // "Ilimitado" IA OCR
				
				// Office "ilimitado"
				OfficePagesPerDay:   1000,  // "Ilimitado" Office
				OfficePagesPerMonth: 25000, // "Ilimitado" Office
				OfficeHasWatermark: false,  // ❌ Sin marca de agua
				
				MaxBytesPerDay:   10 * 1024 * 1024 * 1024,  // 10GB por día "ilimitado"
				MaxBytesPerMonth: 500 * 1024 * 1024 * 1024, // 500GB por mes "ilimitado"
				
				// Configuración máxima visible
				RateLimit:         1000,               // 1000 req/min "ilimitado"
				Priority:          10,                 // Prioridad máxima
				ProcessingTimeout: 300 * time.Second, // 5min timeout "ilimitado"
				SpeedLevel:        "turbo",            // Velocidad turbo
				
				// Features corporativas completas
				EnableAIOCR:         true,              // ✅ IA OCR completo
				EnablePriority:      true,              // ✅ Prioridad máxima
				EnableAnalytics:     true,              // ✅ Analytics empresariales
				EnableTeamAccess:    true,              // ✅ Equipos grandes
				EnableAPI:           true,              // ✅ API + webhooks completos
				EnableAdvancedFeats: true,              // ✅ Todas las features
				HasWatermark:        false,             // ❌ Sin marca de agua
				HasAds:              false,             // ❌ Sin publicidad
				SupportLevel:        "dedicated",       // Soporte dedicado + SLA
				MaxTeamUsers:        100,               // 100 usuarios en equipo
				
				// Límites internos invisibles estrictos para proteger servidor
				InternalLimits: &InternalLimits{
					AbsoluteMaxFileSize:   350 * 1024 * 1024, // 350MB absoluto (invisible)
					AbsoluteMaxQueueSize:  50,                 // 50 jobs globales máximo
					AbsoluteMaxCPUPercent: 85,                 // 85% CPU máximo
					AbsoluteMaxRAMPercent: 80,                 // 80% RAM máximo
					
					ContainerCPULimit:   4.0,          // 4.0 CPU para engine (máximo)
					ContainerRAMLimit:   2048 << 20,   // 2GB RAM para engine
					ContainerCPUReserve: 1.0,          // 1.0 CPU reservado
					ContainerRAMReserve: 512 << 20,    // 512MB RAM reservado
					
					OCRTimeoutSeconds:    60,          // 60s máximo OCR (estricto)
					OfficeTimeoutSeconds: 120,         // 120s máximo Office (estricto)
					PDFTimeoutSeconds:    45,          // 45s máximo PDF (estricto)
					
					QueuePauseThreshold:  50,          // Pausar si cola > 50
					HighLoadFileLimit:    200 << 20,   // 200MB en carga alta
					ProtectorModeSeconds: 300,         // 5 min modo protector
					
					HealthCheckInterval: 10 * time.Second, // Health check cada 10s
					AutoRestartEnabled:  true,             // Auto restart habilitado
					MaxWorkerHangTime:   30 * time.Second, // 30s antes restart
				},
			},
		},
		Pricing: map[Plan]PlanPricing{
			PlanFree: {
				Monthly: 0.0,
				Yearly:  0.0,
			},
			PlanPremium: {
				Monthly: 9.99,
				Yearly:  99.99, // 2 meses gratis
			},
			PlanPro: {
				Monthly: 29.99,
				Yearly:  299.99, // 2 meses gratis
			},
			PlanCorporate: {
				Monthly: 199.99,
				Yearly:  1999.99, // 2 meses gratis
			},
		},
	}
}

// GetPlanLimits obtiene los límites para un plan específico
func (pc *PlanConfiguration) GetPlanLimits(plan Plan) PlanLimits {
	if limits, exists := pc.Plans[plan]; exists {
		return limits
	}
	return pc.Plans[PlanFree] // Retornar plan free como default
}

// GetPlanPricing obtiene los precios para un plan específico
func (pc *PlanConfiguration) GetPlanPricing(plan Plan) PlanPricing {
	if pricing, exists := pc.Pricing[plan]; exists {
		return pricing
	}
	return pc.Pricing[PlanFree] // Retornar pricing free como default
}