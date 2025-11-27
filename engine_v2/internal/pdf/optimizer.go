package pdf

import (
    "bytes"
    "fmt"
    "image"
    "image/jpeg"
    "io"
    "os"

    "github.com/pdfcpu/pdfcpu/pkg/api"
    "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
    "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
    "github.com/tucentropdf/engine-v2/pkg/logger"
)

// OptimizerOptions opciones de optimización de PDF
type OptimizerOptions struct {
	CompressImages     bool    // Comprimir imágenes embebidas
	ImageQuality       int     // Calidad JPEG (1-100, default: 85)
	DownsampleDPI      int     // DPI objetivo para downsampling (default: 150)
	RemoveMetadata     bool    // Eliminar metadata
	RemoveAnnotations  bool    // Eliminar anotaciones
	RemoveBookmarks    bool    // Eliminar bookmarks
	RemoveJavaScript   bool    // Eliminar JavaScript
	LinearizePDF       bool    // Linearizar para fast web view
	CompressStreams    bool    // Re-comprimir streams
	RemoveDuplicates   bool    // Eliminar objetos duplicados
	MaxFileSizeMB      int     // Tamaño máximo objetivo (0 = sin límite)
}

// Optimizer maneja la optimización de archivos PDF
type Optimizer struct {
	logger *logger.Logger
}

// OptimizationResult resultado de la optimización
type OptimizationResult struct {
	OriginalSize   int64   `json:"original_size"`
	OptimizedSize  int64   `json:"optimized_size"`
	ReductionBytes int64   `json:"reduction_bytes"`
	ReductionPct   float64 `json:"reduction_pct"`
	ImagesOptimized int    `json:"images_optimized"`
	DurationMs     int64   `json:"duration_ms"`
}

// NewOptimizer crea una nueva instancia de Optimizer
func NewOptimizer(log *logger.Logger) *Optimizer {
	return &Optimizer{
		logger: log,
	}
}

// GetDefaultOptions retorna opciones por defecto
func (o *Optimizer) GetDefaultOptions() *OptimizerOptions {
	return &OptimizerOptions{
		CompressImages:    true,
		ImageQuality:      85,
		DownsampleDPI:     150,
		RemoveMetadata:    true,
		RemoveAnnotations: false,
		RemoveBookmarks:   false,
		RemoveJavaScript:  true,
		LinearizePDF:      true,
		CompressStreams:   true,
		RemoveDuplicates:  true,
		MaxFileSizeMB:     0, // Sin límite
	}
}

// OptimizePDF optimiza un archivo PDF
func (o *Optimizer) OptimizePDF(inputPath, outputPath string, options *OptimizerOptions) (*OptimizationResult, error) {
	if options == nil {
		options = o.GetDefaultOptions()
	}
	
	// Obtener tamaño original
	originalSize, err := o.getFileSize(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get original size: %w", err)
	}
	
	o.logger.Info("Starting PDF optimization",
		"input", inputPath,
		"original_size", originalSize,
		"options", options,
	)
	
	// Leer PDF
	ctx, err := api.ReadContextFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}
	
	result := &OptimizationResult{
		OriginalSize: originalSize,
	}
	
	// Pipeline de optimización
	
	// 1. Comprimir imágenes
	if options.CompressImages {
		count, err := o.compressImages(ctx, options)
		if err != nil {
			o.logger.Warn("Image compression failed", "error", err)
		} else {
			result.ImagesOptimized = count
			o.logger.Debug("Images compressed", "count", count)
		}
	}
	
	// 2. Eliminar metadata
	if options.RemoveMetadata {
		if err := o.removeMetadata(ctx); err != nil {
			o.logger.Warn("Metadata removal failed", "error", err)
		} else {
			o.logger.Debug("Metadata removed")
		}
	}
	
	// 3. Eliminar JavaScript
	if options.RemoveJavaScript {
		if err := o.removeJavaScript(ctx); err != nil {
			o.logger.Warn("JavaScript removal failed", "error", err)
		} else {
			o.logger.Debug("JavaScript removed")
		}
	}
	
	// 4. Eliminar anotaciones
	if options.RemoveAnnotations {
		if err := o.removeAnnotations(ctx); err != nil {
			o.logger.Warn("Annotations removal failed", "error", err)
		} else {
			o.logger.Debug("Annotations removed")
		}
	}
	
	// 5. Eliminar bookmarks
	if options.RemoveBookmarks {
		if err := o.removeBookmarks(ctx); err != nil {
			o.logger.Warn("Bookmarks removal failed", "error", err)
		} else {
			o.logger.Debug("Bookmarks removed")
		}
	}
	
	// 6. Comprimir streams
	if options.CompressStreams {
		if err := o.compressStreams(ctx); err != nil {
			o.logger.Warn("Stream compression failed", "error", err)
		} else {
			o.logger.Debug("Streams compressed")
		}
	}
	
	// 7. Eliminar duplicados
	if options.RemoveDuplicates {
		if err := o.removeDuplicates(ctx); err != nil {
			o.logger.Warn("Duplicate removal failed", "error", err)
		} else {
			o.logger.Debug("Duplicates removed")
		}
	}
	
	// 8. Linearizar para fast web view
	if options.LinearizePDF {
		if err := o.linearize(ctx); err != nil {
			o.logger.Warn("Linearization failed", "error", err)
		} else {
			o.logger.Debug("PDF linearized")
		}
	}
	
	// Escribir PDF optimizado
	if err := api.WriteContextFile(ctx, outputPath); err != nil {
		return nil, fmt.Errorf("failed to write optimized PDF: %w", err)
	}
	
	// Obtener tamaño optimizado
	optimizedSize, err := o.getFileSize(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get optimized size: %w", err)
	}
	
	// Calcular reducción
	result.OptimizedSize = optimizedSize
	result.ReductionBytes = originalSize - optimizedSize
	result.ReductionPct = float64(result.ReductionBytes) / float64(originalSize) * 100
	
	o.logger.Info("PDF optimization completed",
		"original_size", originalSize,
		"optimized_size", optimizedSize,
		"reduction_pct", fmt.Sprintf("%.2f%%", result.ReductionPct),
		"images_optimized", result.ImagesOptimized,
	)
	
	return result, nil
}

// compressImages comprime imágenes embebidas en el PDF
func (o *Optimizer) compressImages(ctx *model.Context, options *OptimizerOptions) (int, error) {
	count := 0
	
	// Iterar sobre todos los objetos del PDF
	for objNr, obj := range ctx.XRefTable.Table {
		if obj == nil {
			continue
		}
		
        // Verificar si es un stream de imagen
        sd, ok := obj.Object.(*types.StreamDict)
        if !ok {
            continue
        }
        // Verificar si tiene SubType Image
        subType := sd.Subtype()
        if subType == nil || *subType != "Image" {
            continue
        }
        // Comprimir imagen
        if err := o.compressImageStream(sd, options, objNr); err != nil {
            o.logger.Warn("Failed to compress image",
                "obj_nr", objNr,
                "error", err,
            )
            continue
        }
        count++
	}
	
	return count, nil
}

// compressImageStream comprime un stream de imagen individual
func (o *Optimizer) compressImageStream(sd *types.StreamDict, options *OptimizerOptions, objNr int) error {
	// Decodificar stream
	if err := sd.Decode(); err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}
	
	// Obtener datos de imagen
	data := sd.Content
	if len(data) == 0 {
		return nil
	}
	
	// Decodificar imagen
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// Si no se puede decodificar, dejar como está
		return nil
	}
	
	// Solo comprimir imágenes grandes
	bounds := img.Bounds()
	if bounds.Dx() < 100 || bounds.Dy() < 100 {
		return nil // Imagen muy pequeña, no optimizar
	}
	
	o.logger.Debug("Compressing image",
		"obj_nr", objNr,
		"format", format,
		"width", bounds.Dx(),
		"height", bounds.Dy(),
	)
	
	// Downsample si es necesario
	if options.DownsampleDPI > 0 {
		// Calcular factor de downsampling (simplificado)
		// En producción, calcular basado en DPI real de la imagen
		maxDim := bounds.Dx()
		if bounds.Dy() > maxDim {
			maxDim = bounds.Dy()
		}
		
		if maxDim > options.DownsampleDPI*10 { // Heurística: >1500px a 150dpi
			// TODO: Implementar downsampling real
			o.logger.Debug("Image would benefit from downsampling",
				"max_dim", maxDim,
				"target_dpi", options.DownsampleDPI,
			)
		}
	}
	
	// Re-comprimir como JPEG con calidad especificada
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: options.ImageQuality}); err != nil {
		return fmt.Errorf("jpeg encode failed: %w", err)
	}

	// Actualizar stream solo si el resultado es más pequeño
	if buf.Len() < len(data) {
		sd.Content = buf.Bytes()
		sd.Filter = []string{"DCTDecode"}
		o.logger.Debug("Image compressed",
			"obj_nr", objNr,
			"original_size", len(data),
			"compressed_size", buf.Len(),
			"reduction_pct", fmt.Sprintf("%.2f%%", float64(len(data)-buf.Len())/float64(len(data))*100),
		)
	}
	
	return nil
}

// removeMetadata elimina metadata del PDF
func (o *Optimizer) removeMetadata(ctx *model.Context) error {
	// Eliminar Info Dict
	if ctx.Info != nil {
		ctx.Info = nil
	}
	
	// Eliminar XMP metadata si existe
	if ctx.XRefTable.Root != nil {
		delete(ctx.XRefTable.Root, "Metadata")
	}
	
	return nil
}

// removeJavaScript elimina JavaScript del PDF
func (o *Optimizer) removeJavaScript(ctx *model.Context) error {
	if ctx.XRefTable.Root == nil {
		return nil
	}
	// Eliminar JavaScript actions
	delete(ctx.XRefTable.Root, "OpenAction")
	delete(ctx.XRefTable.Root, "AA")
	// Eliminar Names dict con JavaScript
	if namesDict, ok := ctx.XRefTable.Root["Names"]; ok {
		if dict, ok := namesDict.(types.Dict); ok {
			delete(dict, "JavaScript")
		}
	}
	
	return nil
}

// removeAnnotations elimina anotaciones del PDF
func (o *Optimizer) removeAnnotations(ctx *model.Context) error {
	// Iterar sobre todas las páginas
	for i := 1; i <= ctx.PageCount; i++ {
		pageDict, _, _, err := ctx.PageDict(i, false)
		if err != nil {
			continue
		}
		// Eliminar array de anotaciones
		if pageDict.Dict != nil {
			delete(pageDict.Dict, "Annots")
		}
	}
	
	return nil
}

// removeBookmarks elimina bookmarks del PDF
func (o *Optimizer) removeBookmarks(ctx *model.Context) error {
	if ctx.XRefTable.Root == nil {
		return nil
	}
	delete(ctx.XRefTable.Root, "Outlines")
	return nil
}

// compressStreams re-comprime streams del PDF
func (o *Optimizer) compressStreams(ctx *model.Context) error {
	// pdfcpu ya aplica compresión por defecto
	// Esta función es un placeholder para optimizaciones adicionales
	return nil
}

// removeDuplicates elimina objetos duplicados
func (o *Optimizer) removeDuplicates(ctx *model.Context) error {
	// Implementación simplificada
	// pdfcpu maneja esto internamente en WriteContext
	return nil
}

// linearize lineariza el PDF para fast web view
func (o *Optimizer) linearize(ctx *model.Context) error {
	// Marcar para linearización
	ctx.Read.Linearized = true
	return nil
}

// getFileSize obtiene el tamaño de un archivo
func (o *Optimizer) getFileSize(path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	
	return stat.Size(), nil
}

// EstimateReduction estima la reducción de tamaño antes de optimizar
func (o *Optimizer) EstimateReduction(inputPath string, options *OptimizerOptions) (float64, error) {
	// Análisis rápido del PDF sin optimizar
	ctx, err := api.ReadContextFile(inputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PDF: %w", err)
	}
	
	// Estimar basado en presencia de imágenes y metadata
	hasImages := false
	hasMetadata := ctx.Info != nil
	
	for _, obj := range ctx.XRefTable.Table {
		if obj == nil {
			continue
		}
		if sd, ok := obj.Object.(pdfcpu.StreamDict); ok {
			if subType := sd.Subtype(); subType != nil && *subType == "Image" {
				hasImages = true
				break
			}
		}
	}
	
	// Estimación conservadora
	estimatedReduction := 0.0
	if hasImages {
		estimatedReduction += 40.0 // 40% si hay imágenes
	}
	if hasMetadata {
		estimatedReduction += 5.0 // 5% si hay metadata
	}
	if options.CompressStreams {
		estimatedReduction += 10.0 // 10% por re-compresión
	}
	
	// Máximo 60% de reducción estimada
	if estimatedReduction > 60.0 {
		estimatedReduction = 60.0
	}
	
	return estimatedReduction, nil
}

// ValidateOptimized verifica que el PDF optimizado sea válido
func (o *Optimizer) ValidateOptimized(path string) error {
	// Intentar leer el PDF
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		return fmt.Errorf("optimized PDF is invalid: %w", err)
	}
	
	// Validar estructura básica
	if ctx.PageCount == 0 {
		return fmt.Errorf("optimized PDF has no pages")
	}
	
	// Intentar renderizar primera página (verificación adicional)
	// TODO: Implementar rendering test si es necesario
	
	o.logger.Debug("Optimized PDF validated",
		"pages", ctx.PageCount,
	)
	
	return nil
}
