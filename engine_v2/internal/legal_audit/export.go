package legal_audit

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// ExportManager gestiona exportación de evidencia legal
type ExportManager struct {
	db               *LegalAuditLog
	encryptor        *LegalEncryptor
	integrityManager *IntegrityManager
	logger           *logger.Logger
}

// NewExportManager crea nueva instancia del gestor de exportación
func NewExportManager(db interface{}, encryptor *LegalEncryptor, integrityManager *IntegrityManager, log *logger.Logger) *ExportManager {
	return &ExportManager{
		encryptor:        encryptor,
		integrityManager: integrityManager,
		logger:           log,
	}
}

// ExportToFile exporta registros de auditoría a archivo para evidencia legal
func (em *ExportManager) ExportToFile(filter *AuditFilter, request *ExportRequest) (*ExportResult, error) {
	em.logger.Info("📤 Starting legal evidence export",
		"format", request.Format,
		"encrypted", request.Encrypted,
		"admin_id", request.AdminID)

	start := time.Now()
	exportID := uuid.New().String()

	// Crear directorio temporal para exportación
	tempDir := filepath.Join(os.TempDir(), "legal_exports", exportID)
	if err := os.MkdirAll(tempDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create export directory: %w", err)
	}
	defer os.RemoveAll(tempDir) // Limpiar al final

	// Obtener registros a exportar (sin límite)
	filter.Limit = 0 // Remover límite para exportación completa
	records, err := em.getRecordsForExport(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get records for export: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no records found matching the specified criteria")
	}

	// Verificar integridad antes de exportar
	integrityReport := em.integrityManager.VerifyBatchIntegrity(records)
	if !integrityReport.Verified {
		em.logger.Warn("Integrity verification failed for some records",
			"total", integrityReport.RecordCount,
			"valid", integrityReport.Summary.ValidRecords,
			"invalid", integrityReport.Summary.InvalidRecords)
	}

	// Exportar según formato solicitado
	var exportPath string
	switch request.Format {
	case FormatJSON:
		exportPath, err = em.exportToJSON(tempDir, exportID, records, integrityReport)
	case FormatCSV:
		exportPath, err = em.exportToCSV(tempDir, exportID, records, integrityReport)
	case FormatXML:
		exportPath, err = em.exportToXML(tempDir, exportID, records, integrityReport)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", request.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to export to %s: %w", request.Format, err)
	}

	// Crear directorio de destino final
	finalDir := filepath.Join("/var/tucentropdf/exports/legal", time.Now().Format("2006/01"))
	if err := os.MkdirAll(finalDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create final export directory: %w", err)
	}

	// Mover archivo a ubicación final
	finalFileName := fmt.Sprintf("legal_audit_export_%s_%s.%s",
		exportID[:8], time.Now().Format("20060102_150405"), request.Format)
	finalPath := filepath.Join(finalDir, finalFileName)

	if err := em.moveFile(exportPath, finalPath); err != nil {
		return nil, fmt.Errorf("failed to move export file: %w", err)
	}

	// Obtener información del archivo
	fileInfo, err := os.Stat(finalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get export file info: %w", err)
	}

	// Cifrar archivo si se solicitó
	var encryptedPath string
	if request.Encrypted {
		encryptedPath, err = em.encryptor.EncryptExportFile(finalPath)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt export file: %w", err)
		}
		// Eliminar archivo sin cifrar
		os.Remove(finalPath)
		finalPath = encryptedPath
		
		// Actualizar información del archivo
		if fileInfo, err = os.Stat(finalPath); err != nil {
			return nil, fmt.Errorf("failed to get encrypted file info: %w", err)
		}
	}

	// Crear token de descarga
	downloadToken := em.generateDownloadToken(exportID, request.AdminID)

	// Crear resultado
	result := &ExportResult{
		ExportID:         exportID,
		RecordCount:      len(records),
		FilePath:         finalPath,
		FileSize:         fileInfo.Size(),
		Encrypted:        request.Encrypted,
		IntegrityReport:  *integrityReport,
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(7 * 24 * time.Hour), // Expira en 7 días
		DownloadToken:    downloadToken,
	}

	em.logger.Info("Legal evidence export completed",
		"export_id", exportID,
		"records", len(records),
		"file_size", fileInfo.Size(),
		"duration", time.Since(start),
		"integrity_score", integrityReport.IntegrityScore)

	return result, nil
}

// exportToJSON exporta registros a formato JSON
func (em *ExportManager) exportToJSON(dir, exportID string, records []LegalAuditLog, integrityReport *IntegrityReport) (string, error) {
	exportData := struct {
		ExportInfo struct {
			ID               string    `json:"id"`
			CreatedAt        time.Time `json:"created_at"`
			Format           string    `json:"format"`
			TotalRecords     int       `json:"total_records"`
			IntegrityVerified bool     `json:"integrity_verified"`
		} `json:"export_info"`
		IntegrityReport *IntegrityReport `json:"integrity_report"`
		Records         []LegalAuditLog   `json:"records"`
	}{}

	// Llenar información de exportación
	exportData.ExportInfo.ID = exportID
	exportData.ExportInfo.CreatedAt = time.Now()
	exportData.ExportInfo.Format = "JSON"
	exportData.ExportInfo.TotalRecords = len(records)
	exportData.ExportInfo.IntegrityVerified = integrityReport.Verified

	// Incluir reporte de integridad
	exportData.IntegrityReport = integrityReport

	// Incluir registros
	exportData.Records = records

	// Crear archivo JSON
	filePath := filepath.Join(dir, fmt.Sprintf("export_%s.json", exportID))
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	// Escribir JSON con formato legible
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(exportData); err != nil {
		return "", fmt.Errorf("failed to encode JSON: %w", err)
	}

	return filePath, nil
}

// exportToCSV exporta registros a formato CSV
func (em *ExportManager) exportToCSV(dir, exportID string, records []LegalAuditLog, integrityReport *IntegrityReport) (string, error) {
	filePath := filepath.Join(dir, fmt.Sprintf("export_%s.csv", exportID))
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Escribir header
	header := []string{
		"ID", "UserID", "Tool", "Action", "Plan", "FileSize", "IP", "UserAgent",
		"Status", "Reason", "Timestamp", "Abuse", "CompanyID", "APIKeyID", "AdminID",
		"WorkerID", "Duration", "Domain", "IntegrityHash", "Signature", "CreatedAt",
	}
	if err := writer.Write(header); err != nil {
		return "", fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Escribir registros
	for _, record := range records {
		row := []string{
			record.ID.String(),
			em.formatNullableInt64(record.UserID),
			record.Tool,
			record.Action,
			record.Plan,
			em.formatNullableInt64(record.FileSize),
			record.IP,
			record.UserAgent,
			record.Status,
			em.formatNullableString(record.Reason),
			record.Timestamp.Format(time.RFC3339),
			strconv.FormatBool(record.Abuse),
			em.formatNullableInt64(record.CompanyID),
			em.formatNullableString(record.APIKeyID),
			em.formatNullableInt64(record.AdminID),
			em.getMetadataString(record.Metadata, "worker_id"),
			em.getMetadataString(record.Metadata, "duration_ms"),
			em.getMetadataString(record.Metadata, "domain"),
			record.IntegrityHash,
			record.Signature,
			record.CreatedAt.Format(time.RFC3339),
		}

		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	// Escribir información de integridad al final
	writer.Write([]string{}) // Línea vacía
	writer.Write([]string{"=== INTEGRITY REPORT ==="})
	writer.Write([]string{"Total Records", strconv.Itoa(integrityReport.RecordCount)})
	writer.Write([]string{"Verified", strconv.FormatBool(integrityReport.Verified)})
	writer.Write([]string{"Integrity Score", fmt.Sprintf("%.2f%%", integrityReport.IntegrityScore)})
	writer.Write([]string{"Verification Date", integrityReport.VerificationDate.Format(time.RFC3339)})

	return filePath, nil
}

// exportToXML exporta registros a formato XML
func (em *ExportManager) exportToXML(dir, exportID string, records []LegalAuditLog, integrityReport *IntegrityReport) (string, error) {
	type XMLExport struct {
		XMLName xml.Name `xml:"legal_audit_export"`
		ExportInfo struct {
			ID            string    `xml:"id"`
			CreatedAt     time.Time `xml:"created_at"`
			Format        string    `xml:"format"`
			TotalRecords  int       `xml:"total_records"`
		} `xml:"export_info"`
		IntegrityReport struct {
			Verified       bool      `xml:"verified"`
			IntegrityScore float64   `xml:"integrity_score"`
			RecordCount    int       `xml:"record_count"`
			VerifiedAt     time.Time `xml:"verified_at"`
		} `xml:"integrity_report"`
		Records struct {
			Record []LegalAuditLog `xml:"record"`
		} `xml:"records"`
	}

	exportData := XMLExport{}
	
	// Llenar información de exportación
	exportData.ExportInfo.ID = exportID
	exportData.ExportInfo.CreatedAt = time.Now()
	exportData.ExportInfo.Format = "XML"
	exportData.ExportInfo.TotalRecords = len(records)

	// Llenar reporte de integridad
	exportData.IntegrityReport.Verified = integrityReport.Verified
	exportData.IntegrityReport.IntegrityScore = integrityReport.IntegrityScore
	exportData.IntegrityReport.RecordCount = integrityReport.RecordCount
	exportData.IntegrityReport.VerifiedAt = integrityReport.VerificationDate

	// Incluir registros
	exportData.Records.Record = records

	// Crear archivo XML
	filePath := filepath.Join(dir, fmt.Sprintf("export_%s.xml", exportID))
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create XML file: %w", err)
	}
	defer file.Close()

	// Escribir XML con formato
	file.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\\n")
	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")
	if err := encoder.Encode(exportData); err != nil {
		return "", fmt.Errorf("failed to encode XML: %w", err)
	}

	return filePath, nil
}

// getRecordsForExport obtiene registros para exportación (implementación simulada)
func (em *ExportManager) getRecordsForExport(filter *AuditFilter) ([]LegalAuditLog, error) {
	// Esta función debería obtener registros de la base de datos
	// Por ahora retornamos un slice vacío como placeholder
	em.logger.Debug("Getting records for export", "filter", filter)
	
	// TODO: Implementar consulta real a la base de datos
	// En una implementación real, esto sería algo como:
	// return em.service.GetAuditLogs(filter)
	
	return []LegalAuditLog{}, nil
}

// generateDownloadToken genera token seguro para descarga
func (em *ExportManager) generateDownloadToken(exportID string, adminID int64) string {
	// Implementación básica - en producción usar JWT o similar
	tokenData := fmt.Sprintf("%s:%d:%d", exportID, adminID, time.Now().Unix())
	return fmt.Sprintf("%x", []byte(tokenData))
}

// moveFile mueve archivo de origen a destino
func (em *ExportManager) moveFile(src, dst string) error {
	// Copiar archivo
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Eliminar archivo original
	return os.Remove(src)
}

// Funciones helper para formateo

func (em *ExportManager) formatNullableInt64(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func (em *ExportManager) formatNullableString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (em *ExportManager) getMetadataString(metadata *JSONBMetadata, field string) string {
	if metadata == nil {
		return ""
	}

	switch field {
	case "worker_id":
		return metadata.WorkerID
	case "duration_ms":
		if metadata.Duration > 0 {
			return strconv.FormatInt(metadata.Duration, 10)
		}
		return ""
	case "domain":
		return metadata.Domain
	default:
		if metadata.Extra != nil {
			if value, exists := metadata.Extra[field]; exists {
				return fmt.Sprintf("%v", value)
			}
		}
		return ""
	}
}

// CreateIntegrityPackage crea paquete completo de evidencia con verificación
func (em *ExportManager) CreateIntegrityPackage(filter *AuditFilter, adminID int64) (*IntegrityPackage, error) {
	em.logger.Info("📋 Creating comprehensive integrity package")

	// Obtener registros
	records, err := em.getRecordsForExport(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get records: %w", err)
	}

	// Crear reporte de integridad
	integrityReport := em.integrityManager.VerifyBatchIntegrity(records)

	// Crear manifiesto de integridad
	manifest, err := em.integrityManager.CreateIntegrityManifest(records)
	if err != nil {
		return nil, fmt.Errorf("failed to create integrity manifest: %w", err)
	}

	packageID := uuid.New().String()
	packageDir := filepath.Join("/var/tucentropdf/exports/integrity", packageID)
	if err := os.MkdirAll(packageDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create package directory: %w", err)
	}

	// Exportar datos principales
	request := &ExportRequest{
		Format:    FormatJSON,
		Encrypted: true,
		AdminID:   adminID,
	}
	
	exportResult, err := em.ExportToFile(filter, request)
	if err != nil {
		return nil, fmt.Errorf("failed to export data: %w", err)
	}

	// Crear archivos adicionales del paquete
	manifestPath := filepath.Join(packageDir, "integrity_manifest.json")
	if err := em.writeJSONFile(manifestPath, manifest); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	reportPath := filepath.Join(packageDir, "integrity_report.json")
	if err := em.writeJSONFile(reportPath, integrityReport); err != nil {
		return nil, fmt.Errorf("failed to write report: %w", err)
	}

	// Crear archivo README con instrucciones
	readmePath := filepath.Join(packageDir, "README.txt")
	if err := em.createPackageReadme(readmePath, packageID, len(records)); err != nil {
		return nil, fmt.Errorf("failed to create README: %w", err)
	}

	pkg := &IntegrityPackage{
		ID:               packageID,
		RecordCount:      len(records),
		IntegrityScore:   integrityReport.IntegrityScore,
		Verified:         integrityReport.Verified,
		DataExportPath:   exportResult.FilePath,
		ManifestPath:     manifestPath,
		ReportPath:       reportPath,
		ReadmePath:       readmePath,
		CreatedAt:        time.Now(),
		CreatedBy:        adminID,
	}

	return pkg, nil
}

// IntegrityPackage paquete completo de evidencia legal
type IntegrityPackage struct {
	ID             string    `json:"id"`
	RecordCount    int       `json:"record_count"`
	IntegrityScore float64   `json:"integrity_score"`
	Verified       bool      `json:"verified"`
	DataExportPath string    `json:"data_export_path"`
	ManifestPath   string    `json:"manifest_path"`
	ReportPath     string    `json:"report_path"`
	ReadmePath     string    `json:"readme_path"`
	CreatedAt      time.Time `json:"created_at"`
	CreatedBy      int64     `json:"created_by"`
}

// writeJSONFile escribe objeto a archivo JSON
func (em *ExportManager) writeJSONFile(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// createPackageReadme crea archivo README para el paquete
func (em *ExportManager) createPackageReadme(path, packageID string, recordCount int) error {
	content := fmt.Sprintf(`TUCENTROPDF ENGINE V2 - LEGAL AUDIT EVIDENCE PACKAGE
=========================================================

Package ID: %s
Generated: %s
Record Count: %d

This package contains legally verifiable audit evidence with:

1. data_export.json.enc - Encrypted audit records
2. integrity_manifest.json - Cryptographic integrity manifest  
3. integrity_report.json - Verification report
4. README.txt - This file

VERIFICATION INSTRUCTIONS:
1. Decrypt data_export.json.enc using provided key
2. Verify each record hash matches integrity_manifest.json
3. Validate signatures using HMAC-SHA256
4. Cross-reference with integrity_report.json

This package provides cryptographically verifiable evidence 
suitable for legal proceedings and compliance audits.

Generated by TuCentroPDF Engine V2 Legal Audit System
For support: legal@tucentropdf.com
`, packageID, time.Now().Format("2006-01-02 15:04:05 MST"), recordCount)

	return os.WriteFile(path, []byte(content), 0644)
}