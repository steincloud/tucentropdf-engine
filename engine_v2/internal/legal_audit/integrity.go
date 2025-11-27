package legal_audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// IntegrityManager maneja verificación de integridad y firmas digitales
type IntegrityManager struct {
	secretKey []byte
	logger    *logger.Logger
}

// NewIntegrityManager crea nueva instancia del gestor de integridad
func NewIntegrityManager(secretKey string, log *logger.Logger) *IntegrityManager {
	// Usar SHA256 de la clave secreta para consistencia
	hasher := sha256.New()
	hasher.Write([]byte(secretKey))
	derivedKey := hasher.Sum(nil)

	return &IntegrityManager{
		secretKey: derivedKey,
		logger:    log,
	}
}

// GenerateIntegrityHash genera hash SHA256 de un registro completo
func (im *IntegrityManager) GenerateIntegrityHash(record *LegalAuditLog) (string, error) {
	// Crear estructura sin hash ni firma para calcular hash
	recordForHash := struct {
		ID         string             `json:"id"`
		UserID     *int64             `json:"user_id"`
		Tool       string             `json:"tool"`
		Action     string             `json:"action"`
		Plan       string             `json:"plan"`
		FileSize   *int64             `json:"file_size"`
		IP         string             `json:"ip"`
		UserAgent  string             `json:"user_agent"`
		Status     string             `json:"status"`
		Reason     *string            `json:"reason"`
		Timestamp  time.Time          `json:"timestamp"`
		Metadata   *JSONBMetadata     `json:"metadata"`
		Abuse      bool               `json:"abuse"`
		CompanyID  *int64             `json:"company_id"`
		APIKeyID   *string            `json:"api_key_id"`
		AdminID    *int64             `json:"admin_id"`
		CreatedAt  time.Time          `json:"created_at"`
	}{
		ID:        record.ID.String(),
		UserID:    record.UserID,
		Tool:      record.Tool,
		Action:    record.Action,
		Plan:      record.Plan,
		FileSize:  record.FileSize,
		IP:        record.IP,
		UserAgent: record.UserAgent,
		Status:    record.Status,
		Reason:    record.Reason,
		Timestamp: record.Timestamp,
		Metadata:  record.Metadata,
		Abuse:     record.Abuse,
		CompanyID: record.CompanyID,
		APIKeyID:  record.APIKeyID,
		AdminID:   record.AdminID,
		CreatedAt: record.CreatedAt,
	}

	// Serializar a JSON determinístico
	jsonData, err := json.Marshal(recordForHash)
	if err != nil {
		return "", fmt.Errorf("failed to marshal record for hash: %w", err)
	}

	// Generar hash SHA256
	hasher := sha256.New()
	hasher.Write(jsonData)
	hash := hasher.Sum(nil)

	return hex.EncodeToString(hash), nil
}

// GenerateSignature genera firma HMAC-SHA256 del registro
func (im *IntegrityManager) GenerateSignature(record *LegalAuditLog) (string, error) {
	// Obtener hash del registro
	hash, err := im.GenerateIntegrityHash(record)
	if err != nil {
		return "", fmt.Errorf("failed to generate hash for signature: %w", err)
	}

	// Crear estructura para firma incluyendo metadata adicional
	signatureData := struct {
		Hash      string    `json:"hash"`
		Timestamp time.Time `json:"timestamp"`
		Version   string    `json:"version"`
	}{
		Hash:      hash,
		Timestamp: record.CreatedAt,
		Version:   "v2.0",
	}

	// Serializar datos para firma
	jsonData, err := json.Marshal(signatureData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal signature data: %w", err)
	}

	// Generar HMAC-SHA256
	h := hmac.New(sha256.New, im.secretKey)
	h.Write(jsonData)
	signature := h.Sum(nil)

	return hex.EncodeToString(signature), nil
}

// VerifyRecord verifica la integridad y firma de un registro
func (im *IntegrityManager) VerifyRecord(record *LegalAuditLog) (bool, error) {
	im.logger.Debug("Verifying legal audit record integrity", "id", record.ID.String())

	// Verificar hash de integridad
	expectedHash, err := im.GenerateIntegrityHash(record)
	if err != nil {
		return false, fmt.Errorf("failed to generate expected hash: %w", err)
	}

	if record.IntegrityHash != expectedHash {
		im.logger.Error("Integrity hash mismatch",
			"record_id", record.ID.String(),
			"expected_hash", expectedHash,
			"actual_hash", record.IntegrityHash)
		return false, nil
	}

	// Verificar firma digital
	expectedSignature, err := im.GenerateSignature(record)
	if err != nil {
		return false, fmt.Errorf("failed to generate expected signature: %w", err)
	}

	if record.Signature != expectedSignature {
		im.logger.Error("Digital signature mismatch",
			"record_id", record.ID.String(),
			"expected_signature", expectedSignature[:16]+"...",
			"actual_signature", record.Signature[:16]+"...")
		return false, nil
	}

	im.logger.Debug("Record integrity verification successful", "id", record.ID.String())
	return true, nil
}

// VerifyBatchIntegrity verifica integridad de múltiples registros
func (im *IntegrityManager) VerifyBatchIntegrity(records []LegalAuditLog) *IntegrityReport {
	report := &IntegrityReport{
		RecordCount:      len(records),
		Verified:         true,
		VerificationDate: time.Now(),
		FailedRecords:    []string{},
		Hashes:           make([]string, len(records)),
		SignaturesValid:  true,
		IntegrityScore:   0,
		Summary: IntegritySummary{
			TotalRecords:     len(records),
			ValidRecords:     0,
			InvalidRecords:   0,
			CorruptedRecords: 0,
			MissingRecords:   0,
		},
	}

	// Establecer rango de fechas si hay registros
	if len(records) > 0 {
		minTime := records[0].CreatedAt
		maxTime := records[0].CreatedAt

		for _, record := range records {
			if record.CreatedAt.Before(minTime) {
				minTime = record.CreatedAt
			}
			if record.CreatedAt.After(maxTime) {
				maxTime = record.CreatedAt
			}
		}

		report.DateRange = DateRange{
			From: minTime,
			To:   maxTime,
		}
	}

	// Verificar cada registro
	for i, record := range records {
		valid, err := im.VerifyRecord(&record)
		if err != nil {
			im.logger.Error("Error verifying record", "id", record.ID.String(), "error", err)
			report.FailedRecords = append(report.FailedRecords, record.ID.String())
			report.Summary.CorruptedRecords++
			report.Verified = false
			report.SignaturesValid = false
		} else if !valid {
			im.logger.Warn("Invalid record found", "id", record.ID.String())
			report.FailedRecords = append(report.FailedRecords, record.ID.String())
			report.Summary.InvalidRecords++
			report.Verified = false
			report.SignaturesValid = false
		} else {
			report.Summary.ValidRecords++
		}

		// Agregar hash al reporte
		report.Hashes[i] = record.IntegrityHash
	}

	// Calcular puntuación de integridad
	if report.RecordCount > 0 {
		report.IntegrityScore = float64(report.Summary.ValidRecords) / float64(report.RecordCount) * 100
	}

	im.logger.Info("Batch integrity verification completed",
		"total_records", report.RecordCount,
		"valid_records", report.Summary.ValidRecords,
		"invalid_records", report.Summary.InvalidRecords,
		"integrity_score", report.IntegrityScore)

	return report
}

// GenerateRecordChecksum genera checksum adicional para verificación cruzada
func (im *IntegrityManager) GenerateRecordChecksum(record *LegalAuditLog) (string, error) {
	// Combinar campos críticos para checksum
	criticalData := fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		record.ID.String(),
		record.Tool,
		record.Action,
		record.Status,
		record.IP,
		record.Timestamp.Unix())

	// Generar hash SHA256
	hasher := sha256.New()
	hasher.Write([]byte(criticalData))
	checksum := hasher.Sum(nil)

	return hex.EncodeToString(checksum), nil
}

// ValidateRecordStructure valida la estructura básica de un registro
func (im *IntegrityManager) ValidateRecordStructure(record *LegalAuditLog) error {
	// Validaciones básicas requeridas
	if record.ID.String() == "" {
		return fmt.Errorf("record ID is required")
	}

	if record.Tool == "" {
		return fmt.Errorf("tool is required")
	}

	if record.Action == "" {
		return fmt.Errorf("action is required")
	}

	if record.Status == "" {
		return fmt.Errorf("status is required")
	}

	if record.IP == "" {
		return fmt.Errorf("IP address is required")
	}

	if record.IntegrityHash == "" {
		return fmt.Errorf("integrity hash is required")
	}

	if record.Signature == "" {
		return fmt.Errorf("digital signature is required")
	}

	// Validar formato de hash (debe ser hex de 64 caracteres para SHA256)
	if len(record.IntegrityHash) != 64 {
		return fmt.Errorf("invalid integrity hash format")
	}

	// Validar formato de firma (debe ser hex de 64 caracteres para HMAC-SHA256)
	if len(record.Signature) != 64 {
		return fmt.Errorf("invalid signature format")
	}

	return nil
}

// GenerateChainHash genera hash de cadena para secuencia de registros
func (im *IntegrityManager) GenerateChainHash(records []LegalAuditLog) (string, error) {
	if len(records) == 0 {
		return "", fmt.Errorf("no records provided for chain hash")
	}

	// Concatenar hashes de todos los registros en orden temporal
	var hashChain string
	for _, record := range records {
		hashChain += record.IntegrityHash
	}

	// Generar hash de la cadena completa
	hasher := sha256.New()
	hasher.Write([]byte(hashChain))
	chainHash := hasher.Sum(nil)

	return hex.EncodeToString(chainHash), nil
}

// VerifyChainIntegrity verifica integridad de cadena de registros
func (im *IntegrityManager) VerifyChainIntegrity(records []LegalAuditLog, expectedChainHash string) (bool, error) {
	actualChainHash, err := im.GenerateChainHash(records)
	if err != nil {
		return false, err
	}

	valid := actualChainHash == expectedChainHash
	
	if !valid {
		im.logger.Error("Chain integrity verification failed",
			"expected_hash", expectedChainHash,
			"actual_hash", actualChainHash,
			"record_count", len(records))
	}

	return valid, nil
}

// CreateIntegrityManifest crea manifiesto de integridad para exportación
func (im *IntegrityManager) CreateIntegrityManifest(records []LegalAuditLog) (*IntegrityManifest, error) {
	manifest := &IntegrityManifest{
		Version:       "2.0",
		CreatedAt:     time.Now(),
		RecordCount:   len(records),
		RecordHashes:  make([]RecordHashEntry, len(records)),
		Algorithm:     "HMAC-SHA256",
	}

	// Generar hashes y firmas para cada registro
	for i, record := range records {
		manifest.RecordHashes[i] = RecordHashEntry{
			ID:            record.ID.String(),
			IntegrityHash: record.IntegrityHash,
			Signature:     record.Signature,
			Timestamp:     record.CreatedAt,
		}
	}

	// Generar hash de cadena
	chainHash, err := im.GenerateChainHash(records)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chain hash: %w", err)
	}
	manifest.ChainHash = chainHash

	// Generar hash del manifiesto
	manifestHash, err := im.generateManifestHash(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to generate manifest hash: %w", err)
	}
	manifest.ManifestHash = manifestHash

	return manifest, nil
}

// IntegrityManifest manifiesto de integridad para exportación
type IntegrityManifest struct {
	Version       string              `json:"version"`
	CreatedAt     time.Time           `json:"created_at"`
	RecordCount   int                 `json:"record_count"`
	RecordHashes  []RecordHashEntry   `json:"record_hashes"`
	ChainHash     string              `json:"chain_hash"`
	ManifestHash  string              `json:"manifest_hash"`
	Algorithm     string              `json:"algorithm"`
}

// RecordHashEntry entrada de hash para un registro específico
type RecordHashEntry struct {
	ID            string    `json:"id"`
	IntegrityHash string    `json:"integrity_hash"`
	Signature     string    `json:"signature"`
	Timestamp     time.Time `json:"timestamp"`
}

// generateManifestHash genera hash del manifiesto completo
func (im *IntegrityManager) generateManifestHash(manifest *IntegrityManifest) (string, error) {
	// Crear copia sin hash para calcular
	manifestForHash := struct {
		Version       string              `json:"version"`
		CreatedAt     time.Time           `json:"created_at"`
		RecordCount   int                 `json:"record_count"`
		RecordHashes  []RecordHashEntry   `json:"record_hashes"`
		ChainHash     string              `json:"chain_hash"`
		Algorithm     string              `json:"algorithm"`
	}{
		Version:      manifest.Version,
		CreatedAt:    manifest.CreatedAt,
		RecordCount:  manifest.RecordCount,
		RecordHashes: manifest.RecordHashes,
		ChainHash:    manifest.ChainHash,
		Algorithm:    manifest.Algorithm,
	}

	// Serializar y calcular hash
	jsonData, err := json.Marshal(manifestForHash)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	hasher.Write(jsonData)
	hash := hasher.Sum(nil)

	return hex.EncodeToString(hash), nil
}