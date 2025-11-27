package pdf

import (
	"fmt"
	"os"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// PDFUtils utilidades para operaciones con PDF
type PDFUtils struct {
	logger *logger.Logger
}

// NewPDFUtils crear nuevas utilidades PDF
func NewPDFUtils(log *logger.Logger) *PDFUtils {
	return &PDFUtils{
		logger: log,
	}
}

// CountPages contar páginas de un archivo PDF
func (u *PDFUtils) CountPages(filePath string) (int, error) {
	// Verificar que el archivo existe
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return 0, fmt.Errorf("archivo no encontrado: %s", filePath)
	}

	// Configuración por defecto de pdfcpu
	config := model.NewDefaultConfiguration()
	config.ValidationMode = model.ValidationRelaxed

	// Leer información del PDF
	ctx, err := api.ReadContextFile(filePath)
	if err != nil {
		u.logger.Error("Error reading PDF context", 
			"file", filePath, 
			"error", err.Error())
		return 0, fmt.Errorf("error leyendo PDF: %v", err)
	}

	// Obtener número de páginas
	pageCount := ctx.PageCount

	u.logger.Debug("PDF pages counted",
		"file", filePath,
		"pages", pageCount,
	)

	return pageCount, nil
}

// ValidatePDF validar que un archivo sea un PDF válido
func (u *PDFUtils) ValidatePDF(filePath string) error {
	// Verificar que el archivo existe
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("archivo no encontrado: %s", filePath)
	}

	// Configuración por defecto
	config := model.NewDefaultConfiguration()
	config.ValidationMode = model.ValidationRelaxed

	// Validar PDF
	err := api.ValidateFile(filePath, config)
	if err != nil {
		u.logger.Error("PDF validation failed", 
			"file", filePath, 
			"error", err.Error())
		return fmt.Errorf("archivo PDF inválido: %v", err)
	}

	u.logger.Debug("PDF validation successful", "file", filePath)
	return nil
}

// GetPDFInfo obtener información detallada del PDF
func (u *PDFUtils) GetPDFInfo(filePath string) (*PDFInfo, error) {
	// Verificar que el archivo existe
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("archivo no encontrado: %s", filePath)
	}

	// Configuración por defecto
	config := model.NewDefaultConfiguration()
	config.ValidationMode = model.ValidationRelaxed

	// Leer contexto del PDF
	ctx, err := api.ReadContextFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error leyendo PDF: %v", err)
	}

	// Obtener información del archivo
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("error obteniendo información del archivo: %v", err)
	}

	info := &PDFInfo{
		FilePath:     filePath,
		FileSize:     fileInfo.Size(),
		PageCount:    ctx.PageCount,
		Pages:        ctx.PageCount,
		Size:         fileInfo.Size(),
		Version:      ctx.Version().String(),
		Encrypted:    ctx.Encrypt != nil,
		HasBookmarks: len(ctx.Outlines) > 0,
		HasForms:     ctx.Form != nil,
		CreatedAt:    fileInfo.ModTime(),
		ModifiedAt:   fileInfo.ModTime(),
		Metadata:     make(map[string]string),
	}

	// Obtener metadatos si están disponibles
	info.Title = ctx.Title
	info.Author = ctx.Author
	info.Subject = ctx.Subject
	info.Creator = ctx.Creator
	info.Producer = ctx.Producer

	u.logger.Debug("PDF info extracted",
		"file", filePath,
		"pages", info.PageCount,
		"size_bytes", info.FileSize,
		"version", info.Version,
		"encrypted", info.Encrypted,
	)

	return info, nil
}

// PDFInfo información detallada de un PDF
type PDFInfo struct {
	FilePath         string    `json:"file_path"`
	FileSize         int64     `json:"file_size_bytes"`
	PageCount        int       `json:"page_count"`
	Version          string    `json:"pdf_version"`
	Encrypted        bool      `json:"encrypted"`
	HasBookmarks     bool      `json:"has_bookmarks"`
	HasForms         bool      `json:"has_forms"`
	Title            string    `json:"title,omitempty"`
	Author           string    `json:"author,omitempty"`
	Subject          string    `json:"subject,omitempty"`
	Creator          string    `json:"creator,omitempty"`
	Producer         string    `json:"producer,omitempty"`
	CreationDate     string    `json:"creation_date,omitempty"`
	ModificationDate string    `json:"modification_date,omitempty"`
	
	// Campos adicionales para compatibilidad con service.go
	Pages            int                 `json:"pages"`
	Size             int64               `json:"size"`
	CreatedAt        time.Time           `json:"created_at"`
	ModifiedAt       time.Time           `json:"modified_at"`
	Metadata         map[string]string   `json:"metadata"`
}

// EstimateProcessingComplexity estimar complejidad de procesamiento
func (i *PDFInfo) EstimateProcessingComplexity() string {
	if i.PageCount <= 5 {
		return "low"
	} else if i.PageCount <= 20 {
		return "medium"
	} else if i.PageCount <= 100 {
		return "high"
	} else {
		return "very_high"
	}
}

// EstimateOCRCost estimar costo de OCR en páginas
func (i *PDFInfo) EstimateOCRCost() float64 {
	// Costo base por página (estimado)
	baseCostPerPage := 0.01 // $0.01 por página
	
	// Factor de complejidad
	complexityMultiplier := 1.0
	switch i.EstimateProcessingComplexity() {
	case "medium":
		complexityMultiplier = 1.2
	case "high":
		complexityMultiplier = 1.5
	case "very_high":
		complexityMultiplier = 2.0
	}

	// Factor de encriptación
	if i.Encrypted {
		complexityMultiplier *= 1.1
	}

	return float64(i.PageCount) * baseCostPerPage * complexityMultiplier
}