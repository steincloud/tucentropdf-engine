package models

import (
	"time"
	"github.com/google/uuid"
)

// AnalyticsOperation representa una operación registrada para analíticas
type AnalyticsOperation struct {
	ID           uuid.UUID `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	// Datos del usuario
	UserID       string    `json:"user_id" gorm:"index;not null"`
	Plan         string    `json:"plan" gorm:"index;not null"`           // free, premium, pro, corporate
	IsTeamMember bool      `json:"is_team_member" gorm:"default:false"`
	Country      string    `json:"country,omitempty" gorm:"index"`       // país del usuario

	// Datos de la operación
	Tool         string    `json:"tool" gorm:"index;not null"`           // pdf_split, pdf_merge, pdf2word, etc.
	Operation    string    `json:"operation" gorm:"index"`               // operación específica
	FileSize     int64     `json:"file_size"`                            // tamaño del archivo en bytes
	ResultSize   int64     `json:"result_size"`                          // tamaño del resultado en bytes
	Pages        int       `json:"pages"`                                // número de páginas
	Worker       string    `json:"worker" gorm:"index"`                  // api, ocr-worker, office-worker
	Status       string    `json:"status" gorm:"index;not null"`         // success, failed, timeout, canceled, resource_limit
	FailReason   string    `json:"fail_reason,omitempty"`                // motivo de fallo si aplica

	// Datos de rendimiento
	Duration     int64     `json:"duration_ms"`                          // duración total en ms
	CPUUsed      float64   `json:"cpu_used"`                             // CPU usado por worker (%)
	RAMUsed      int64     `json:"ram_used"`                             // RAM usada en bytes
	QueueTime    int64     `json:"queue_time_ms"`                        // tiempo en cola en ms
	Retries      int       `json:"retries" gorm:"default:0"`             // número de reintentos

	// Metadatos
	Timestamp    time.Time `json:"timestamp" gorm:"index;not null"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName especifica el nombre de la tabla
func (AnalyticsOperation) TableName() string {
	return "analytics_operations"
}

// ToolStats estadísticas por herramienta
type ToolStats struct {
	Tool         string  `json:"tool"`
	TotalUsage   int64   `json:"total_usage"`
	SuccessRate  float64 `json:"success_rate"`
	AvgDuration  float64 `json:"avg_duration_ms"`
	AvgFileSize  float64 `json:"avg_file_size"`
	FailureCount int64   `json:"failure_count"`
	LastUsed     time.Time `json:"last_used"`
}

// UserStats estadísticas por usuario
type UserStats struct {
	UserID           string            `json:"user_id"`
	Plan             string            `json:"plan"`
	TotalOperations  int64             `json:"total_operations"`
	DailyOperations  int64             `json:"daily_operations"`
	MonthlyOperations int64             `json:"monthly_operations"`
	TotalMBProcessed float64           `json:"total_mb_processed"`
	ToolBreakdown    map[string]int64  `json:"tool_breakdown"`
	LastActivity     time.Time         `json:"last_activity"`
	IsActive         bool              `json:"is_active"`
}

// PlanStats estadísticas por plan
type PlanStats struct {
	Plan               string            `json:"plan"`
	TotalUsers         int64             `json:"total_users"`
	ActiveUsers        int64             `json:"active_users"`
	TotalOperations    int64             `json:"total_operations"`
	AvgOperationsPerUser float64         `json:"avg_operations_per_user"`
	MostUsedTools      map[string]int64  `json:"most_used_tools"`
	UpgradeOpportunities int64           `json:"upgrade_opportunities"`
	RetentionRate      float64           `json:"retention_rate"`
}

// WorkerStats estadísticas por worker
type WorkerStats struct {
	Worker       string  `json:"worker"`
	TotalJobs    int64   `json:"total_jobs"`
	SuccessRate  float64 `json:"success_rate"`
	AvgDuration  float64 `json:"avg_duration_ms"`
	AvgCPU       float64 `json:"avg_cpu_percent"`
	AvgRAM       float64 `json:"avg_ram_mb"`
	CurrentLoad  float64 `json:"current_load"`
	IsHealthy    bool    `json:"is_healthy"`
}

// SystemOverview vista general del sistema
type SystemOverview struct {
	TotalOperations     int64             `json:"total_operations"`
	OperationsToday     int64             `json:"operations_today"`
	OperationsThisMonth int64             `json:"operations_this_month"`
	ActiveUsers         int64             `json:"active_users"`
	TotalUsers          int64             `json:"total_users"`
	MostPopularTool     string            `json:"most_popular_tool"`
	OverallSuccessRate  float64           `json:"overall_success_rate"`
	AvgProcessingTime   float64           `json:"avg_processing_time_ms"`
	TopFailureReasons   map[string]int64  `json:"top_failure_reasons"`
	PlanDistribution    map[string]int64  `json:"plan_distribution"`
	SystemHealth        string            `json:"system_health"` // healthy, warning, critical
}

// TrendData datos de tendencias
type TrendData struct {
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
	Label string    `json:"label,omitempty"`
}

// UpgradeOpportunity oportunidad de upgrade
type UpgradeOpportunity struct {
	UserID          string    `json:"user_id"`
	CurrentPlan     string    `json:"current_plan"`
	SuggestedPlan   string    `json:"suggested_plan"`
	Reason          string    `json:"reason"`
	Confidence      float64   `json:"confidence"`     // 0-1
	PotentialRevenue float64  `json:"potential_revenue"`
	DetectedAt      time.Time `json:"detected_at"`
}

// BusinessInsight insight de negocio
type BusinessInsight struct {
	Type        string                 `json:"type"` // optimization, revenue, user_behavior, technical
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Severity    string                 `json:"severity"` // low, medium, high, critical
	Data        map[string]interface{} `json:"data"`
	ActionItems []string               `json:"action_items"`
	GeneratedAt time.Time              `json:"generated_at"`
}