package legal_audit

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LegalAuditLog representa un registro inmutable de auditoría legal
type LegalAuditLog struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID        *int64         `json:"user_id" gorm:"index"`
	Tool          string         `json:"tool" gorm:"size:50;not null;index"`
	Action        string         `json:"action" gorm:"size:50;not null;index"`
	Plan          string         `json:"plan" gorm:"size:20;index"`
	FileSize      *int64         `json:"file_size"`
	IP            string         `json:"ip" gorm:"size:45;not null"`
	UserAgent     string         `json:"user_agent" gorm:"type:text"`
	Status        string         `json:"status" gorm:"size:20;not null;index"`
	Reason        *string        `json:"reason" gorm:"type:text"`
	Timestamp     time.Time      `json:"timestamp" gorm:"default:now();index"`
	Metadata      *JSONBMetadata `json:"metadata" gorm:"type:jsonb"`
	IntegrityHash string         `json:"integrity_hash" gorm:"size:128;not null;index"`
	Signature     string         `json:"signature" gorm:"size:256;not null"`
	
	// Campos adicionales para auditoría especializada
	Abuse         bool           `json:"abuse" gorm:"default:false;index"`
	CompanyID     *int64         `json:"company_id" gorm:"index"`
	APIKeyID      *string        `json:"api_key_id" gorm:"size:64;index"`
	AdminID       *int64         `json:"admin_id" gorm:"index"`
	
	// Timestamps adicionales para auditoría forense
	CreatedAt     time.Time      `json:"created_at" gorm:"autoCreateTime"`
}

// JSONBMetadata estructura para metadatos flexibles en JSONB
type JSONBMetadata struct {
	// Información del worker
	WorkerID     string  `json:"worker_id,omitempty"`
	Duration     int64   `json:"duration_ms,omitempty"`
	ProcessingID string  `json:"processing_id,omitempty"`
	
	// Información del archivo
	OriginalName string  `json:"original_name,omitempty"`
	FileType     string  `json:"file_type,omitempty"`
	Pages        int     `json:"pages,omitempty"`
	
	// Información de límites
	LimitType    string  `json:"limit_type,omitempty"`
	LimitValue   int64   `json:"limit_value,omitempty"`
	CurrentUsage int64   `json:"current_usage,omitempty"`
	
	// Información de suscripción
	SubscriptionID     string  `json:"subscription_id,omitempty"`
	PaymentMethod      string  `json:"payment_method,omitempty"`
	Amount             float64 `json:"amount,omitempty"`
	Currency           string  `json:"currency,omitempty"`
	
	// Información de API corporativa
	Domain         string  `json:"domain,omitempty"`
	Endpoint       string  `json:"endpoint,omitempty"`
	RequestMethod  string  `json:"request_method,omitempty"`
	ResponseSize   int64   `json:"response_size,omitempty"`
	
	// Información de abuso/seguridad
	RateLimitHit   bool    `json:"rate_limit_hit,omitempty"`
	VPNDetected    bool    `json:"vpn_detected,omitempty"`
	SuspiciousUA   bool    `json:"suspicious_ua,omitempty"`
	GeoLocation    string  `json:"geo_location,omitempty"`
	
	// Información administrativa
	AdminAction    string  `json:"admin_action,omitempty"`
	TargetUserID   int64   `json:"target_user_id,omitempty"`
	ExportType     string  `json:"export_type,omitempty"`
	
	// Campos adicionales para extensibilidad
	Extra          map[string]interface{} `json:"extra,omitempty"`
}

// AuditEvent estructura para crear nuevos eventos de auditoría
type AuditEvent struct {
	UserID       *int64        `json:"user_id"`
	Tool         string        `json:"tool" validate:"required"`
	Action       string        `json:"action" validate:"required"`
	Plan         string        `json:"plan"`
	FileSize     *int64        `json:"file_size"`
	IP           string        `json:"ip" validate:"required,ip"`
	UserAgent    string        `json:"user_agent"`
	Status       string        `json:"status" validate:"required"`
	Reason       *string       `json:"reason"`
	Metadata     *JSONBMetadata `json:"metadata"`
	Abuse        bool          `json:"abuse"`
	CompanyID    *int64        `json:"company_id"`
	APIKeyID     *string       `json:"api_key_id"`
	AdminID      *int64        `json:"admin_id"`
}

// AuditFilter estructura para filtrar consultas de auditoría
type AuditFilter struct {
	UserID       *int64     `json:"user_id" query:"user_id"`
	CompanyID    *int64     `json:"company_id" query:"company_id"`
	AdminID      *int64     `json:"admin_id" query:"admin_id"`
	Tool         string     `json:"tool" query:"tool"`
	Action       string     `json:"action" query:"action"`
	Plan         string     `json:"plan" query:"plan"`
	Status       string     `json:"status" query:"status"`
	APIKeyID     *string    `json:"api_key_id" query:"api_key_id"`
	FromDate     *time.Time `json:"from_date" query:"from_date"`
	ToDate       *time.Time `json:"to_date" query:"to_date"`
	AbuseOnly    bool       `json:"abuse_only" query:"abuse_only"`
	VerifiedOnly bool       `json:"verified_only" query:"verified_only"`
	Limit        int        `json:"limit" query:"limit"`
	Offset       int        `json:"offset" query:"offset"`
}

// IntegrityReport estructura para reportes de verificación de integridad
type IntegrityReport struct {
	RecordCount       int               `json:"record_count"`
	Verified          bool              `json:"verified"`
	VerificationDate  time.Time         `json:"verification_date"`
	FailedRecords     []string          `json:"failed_records,omitempty"`
	Hashes            []string          `json:"hashes"`
	SignaturesValid   bool              `json:"signatures_valid"`
	DateRange         DateRange         `json:"date_range"`
	IntegrityScore    float64           `json:"integrity_score"`
	Summary           IntegritySummary  `json:"summary"`
}

// DateRange rango de fechas para reportes
type DateRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// IntegritySummary resumen de integridad
type IntegritySummary struct {
	TotalRecords     int `json:"total_records"`
	ValidRecords     int `json:"valid_records"`
	InvalidRecords   int `json:"invalid_records"`
	CorruptedRecords int `json:"corrupted_records"`
	MissingRecords   int `json:"missing_records"`
}

// ExportRequest estructura para solicitudes de exportación legal
type ExportRequest struct {
	UserID       *int64     `json:"user_id" query:"user_id"`
	CompanyID    *int64     `json:"company_id" query:"company_id"`
	FromDate     time.Time  `json:"from_date" query:"from_date" validate:"required"`
	ToDate       time.Time  `json:"to_date" query:"to_date" validate:"required"`
	Tool         string     `json:"tool" query:"tool"`
	Action       string     `json:"action" query:"action"`
	Status       string     `json:"status" query:"status"`
	IncludeAbuse bool       `json:"include_abuse" query:"include_abuse"`
	Format       string     `json:"format" query:"format"` // json, csv, xml
	Encrypted    bool       `json:"encrypted" query:"encrypted"`
	AdminID      int64      `json:"admin_id"`
	RequestIP    string     `json:"request_ip"`
	Reason       string     `json:"reason" validate:"required"`
}

// ExportResult resultado de exportación
type ExportResult struct {
	ExportID         string           `json:"export_id"`
	RecordCount      int              `json:"record_count"`
	FilePath         string           `json:"file_path"`
	FileSize         int64            `json:"file_size"`
	Encrypted        bool             `json:"encrypted"`
	IntegrityReport  IntegrityReport  `json:"integrity_report"`
	CreatedAt        time.Time        `json:"created_at"`
	ExpiresAt        time.Time        `json:"expires_at"`
	DownloadToken    string           `json:"download_token"`
}

// RetentionPolicy política de retención de registros
type RetentionPolicy struct {
	RetentionYears    int    `json:"retention_years"`
	ArchiveThreshold  int    `json:"archive_threshold_months"`
	ArchivePath       string `json:"archive_path"`
	CompressionLevel  int    `json:"compression_level"`
	EncryptArchives   bool   `json:"encrypt_archives"`
}

// ArchiveRecord registro archivado
type ArchiveRecord struct {
	ID            uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	OriginalID    uuid.UUID `json:"original_id" gorm:"type:uuid;not null;index"`
	ArchivePath   string    `json:"archive_path" gorm:"size:500;not null"`
	CompressedSize int64    `json:"compressed_size"`
	OriginalSize   int64    `json:"original_size"`
	Encrypted     bool      `json:"encrypted"`
	ArchiveDate   time.Time `json:"archive_date" gorm:"default:now()"`
	IntegrityHash string    `json:"integrity_hash" gorm:"size:128;not null"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// AuditStats estadísticas de auditoría
type AuditStats struct {
	TotalRecords      int64              `json:"total_records"`
	RecordsByTool     map[string]int64   `json:"records_by_tool"`
	RecordsByAction   map[string]int64   `json:"records_by_action"`
	RecordsByStatus   map[string]int64   `json:"records_by_status"`
	RecordsByPlan     map[string]int64   `json:"records_by_plan"`
	AbuseRecords      int64              `json:"abuse_records"`
	FailedRecords     int64              `json:"failed_records"`
	DateRange         DateRange          `json:"date_range"`
	TopUsers          []UserAuditSummary `json:"top_users"`
	TopCompanies      []CompanyAuditSummary `json:"top_companies"`
}

// UserAuditSummary resumen de auditoría por usuario
type UserAuditSummary struct {
	UserID       int64 `json:"user_id"`
	RecordCount  int64 `json:"record_count"`
	AbuseCount   int64 `json:"abuse_count"`
	FailedCount  int64 `json:"failed_count"`
	LastActivity time.Time `json:"last_activity"`
}

// CompanyAuditSummary resumen de auditoría por empresa
type CompanyAuditSummary struct {
	CompanyID    int64 `json:"company_id"`
	RecordCount  int64 `json:"record_count"`
	APIUsage     int64 `json:"api_usage"`
	AbuseCount   int64 `json:"abuse_count"`
	LastActivity time.Time `json:"last_activity"`
}

// Constantes para auditoría legal
const (
	// Formatos de exportación
	FormatJSON = "json"
	FormatCSV  = "csv"
	FormatXML  = "xml"

	// Tools
	ToolPDFCompress    = "pdf_compress"
	ToolPDFMerge       = "pdf_merge"
	ToolPDFSplit       = "pdf_split"
	ToolPDFExtract     = "pdf_extract"
	ToolOCR            = "ocr"
	ToolOfficeConvert  = "office_convert"
	ToolImageProcess   = "image_process"
	ToolAPI            = "api"
	ToolAdmin          = "admin"
	ToolAuth           = "auth"
	ToolSubscription   = "subscription"
	
	// Actions
	ActionUpload          = "upload"
	ActionProcess         = "process"
	ActionDownload        = "download"
	ActionFail            = "fail"
	ActionLimitExceeded   = "limit_exceeded"
	ActionBlocked         = "blocked"
	ActionLogin           = "login"
	ActionLogout          = "logout"
	ActionSubscribe       = "subscribe"
	ActionUpgrade         = "upgrade"
	ActionDowngrade       = "downgrade"
	ActionCancel          = "cancel"
	ActionAPIKeyCreate    = "api_key_create"
	ActionAPIKeyRevoke    = "api_key_revoke"
	ActionExport          = "export"
	ActionAdminAccess     = "admin_access"
	ActionSystemChange    = "system_change"
	
	// Status
	StatusSuccess         = "success"
	StatusFail            = "fail"
	StatusRejected        = "rejected"
	StatusProtectorBlocked = "protector_blocked"
	StatusTimeout         = "timeout"
	StatusCorrupt         = "corrupt"
	StatusAbuse           = "abuse"
	StatusPending         = "pending"
	StatusExpired         = "expired"
	
	// Plans
	PlanFree       = "free"
	PlanPremium    = "premium"
	PlanPro        = "pro"
	PlanCorporate  = "corporate"
	PlanEnterprise = "enterprise"
)

// BeforeCreate hook para asegurar inmutabilidad
func (l *LegalAuditLog) BeforeCreate(tx *gorm.DB) error {
	// Generar UUID si no existe
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	
	// Establecer timestamp si no existe
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Now()
	}
	
	return nil
}

// TableName especifica el nombre de la tabla
func (LegalAuditLog) TableName() string {
	return "legal_audit_logs"
}

// TableName para ArchiveRecord
func (ArchiveRecord) TableName() string {
	return "legal_audit_archives"
}