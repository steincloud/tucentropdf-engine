package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Preprocessor maneja el pre-procesamiento de imágenes para OCR
type Preprocessor struct {
	logger *logger.Logger
}

// PreprocessingOptions opciones de pre-procesamiento
type PreprocessingOptions struct {
	Deskew         bool    // Corregir rotación
	Denoise        bool    // Eliminar ruido
	EnhanceContrast bool   // Mejorar contraste
	Binarize       bool    // Convertir a blanco/negro
	Upscale        bool    // Aumentar resolución si es muy baja
	TargetDPI      int     // DPI objetivo (default: 300)
	Threshold      uint8   // Umbral para binarización (default: 128)
}

// NewPreprocessor crea una nueva instancia
func NewPreprocessor(log *logger.Logger) *Preprocessor {
	return &Preprocessor{
		logger: log,
	}
}

// ProcessImage procesa una imagen para mejorar accuracy de OCR
func (p *Preprocessor) ProcessImage(inputPath string, options *PreprocessingOptions) (string, error) {
	// Validar opciones
	if options == nil {
		options = p.getDefaultOptions()
	}
	
	// Cargar imagen
	img, format, err := p.loadImage(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to load image: %w", err)
	}
	
	p.logger.Debug("Image loaded",
		"format", format,
		"width", img.Bounds().Dx(),
		"height", img.Bounds().Dy(),
	)
	
	// Pipeline de procesamiento
	processed := img
	
	// 1. Upscale si la resolución es muy baja (<150 DPI estimado)
	if options.Upscale {
		if processed.Bounds().Dx() < 1000 || processed.Bounds().Dy() < 1000 {
			p.logger.Debug("Upscaling low-resolution image")
			processed = p.upscale(processed, 2.0)
		}
	}
	
	// 2. Denoise (reducir ruido)
	if options.Denoise {
		p.logger.Debug("Applying denoising filter")
		processed = p.denoise(processed)
	}
	
	// 3. Enhance contrast (mejorar contraste)
	if options.EnhanceContrast {
		p.logger.Debug("Enhancing contrast")
		processed = p.enhanceContrast(processed)
	}
	
	// 4. Deskew (corregir rotación)
	if options.Deskew {
		p.logger.Debug("Deskewing image")
		angle := p.detectSkewAngle(processed)
		if math.Abs(angle) > 0.5 { // Solo corregir si > 0.5 grados
			processed = p.rotate(processed, angle)
			p.logger.Debug("Image deskewed", "angle", angle)
		}
	}
	
	// 5. Binarize (convertir a blanco/negro)
	if options.Binarize {
		p.logger.Debug("Binarizing image", "threshold", options.Threshold)
		processed = p.binarize(processed, options.Threshold)
	}
	
	// Guardar imagen procesada
	outputPath := p.generateOutputPath(inputPath)
	if err := p.saveImage(processed, outputPath, format); err != nil {
		return "", fmt.Errorf("failed to save processed image: %w", err)
	}
	
	p.logger.Info("Image preprocessed successfully",
		"input", filepath.Base(inputPath),
		"output", filepath.Base(outputPath),
	)
	
	return outputPath, nil
}

// getDefaultOptions retorna opciones por defecto
func (p *Preprocessor) getDefaultOptions() *PreprocessingOptions {
	return &PreprocessingOptions{
		Deskew:         true,
		Denoise:        true,
		EnhanceContrast: true,
		Binarize:       false, // Tesseract funciona mejor con escala de grises
		Upscale:        true,
		TargetDPI:      300,
		Threshold:      128,
	}
}

// loadImage carga una imagen desde archivo
func (p *Preprocessor) loadImage(path string) (image.Image, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", err
	}
	
	return img, format, nil
}

// saveImage guarda una imagen en archivo
func (p *Preprocessor) saveImage(img image.Image, path string, format string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	switch format {
	case "jpeg", "jpg":
		return jpeg.Encode(file, img, &jpeg.Options{Quality: 95})
	case "png":
		return png.Encode(file, img)
	default:
		// Por defecto guardar como PNG
		return png.Encode(file, img)
	}
}

// generateOutputPath genera ruta de salida para imagen procesada
func (p *Preprocessor) generateOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	ext := filepath.Ext(inputPath)
	base := filepath.Base(inputPath)
	nameWithoutExt := base[:len(base)-len(ext)]
	
	return filepath.Join(dir, nameWithoutExt+"_processed"+ext)
}

// upscale aumenta la resolución de la imagen
func (p *Preprocessor) upscale(img image.Image, factor float64) image.Image {
	bounds := img.Bounds()
	newWidth := int(float64(bounds.Dx()) * factor)
	newHeight := int(float64(bounds.Dy()) * factor)
	
	scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	
	// Bilinear interpolation simplificada
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) / factor)
			srcY := int(float64(y) / factor)
			
			// Clamp a bounds
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			if srcY >= bounds.Max.Y {
				srcY = bounds.Max.Y - 1
			}
			
			scaled.Set(x, y, img.At(srcX, srcY))
		}
	}
	
	return scaled
}

// denoise reduce el ruido de la imagen (median filter simplificado)
func (p *Preprocessor) denoise(img image.Image) image.Image {
	bounds := img.Bounds()
	denoised := image.NewRGBA(bounds)
	
	// Aplicar filtro mediano 3x3
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x++ {
			// Obtener vecinos 3x3
			var rVals, gVals, bVals []uint32
			
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					r, g, b, _ := img.At(x+dx, y+dy).RGBA()
					rVals = append(rVals, r)
					gVals = append(gVals, g)
					bVals = append(bVals, b)
				}
			}
			
			// Calcular mediana (aproximación: tomar valor medio)
			rMed := median(rVals)
			gMed := median(gVals)
			bMed := median(bVals)
			
			denoised.Set(x, y, color.RGBA{
				R: uint8(rMed >> 8),
				G: uint8(gMed >> 8),
				B: uint8(bMed >> 8),
				A: 255,
			})
		}
	}
	
	return denoised
}

// enhanceContrast mejora el contraste usando histogram stretching
func (p *Preprocessor) enhanceContrast(img image.Image) image.Image {
	bounds := img.Bounds()
	enhanced := image.NewRGBA(bounds)
	
	// Encontrar min y max de luminosidad
	var minLum, maxLum uint8 = 255, 0
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := uint8((0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 256)
			
			if lum < minLum {
				minLum = lum
			}
			if lum > maxLum {
				maxLum = lum
			}
		}
	}
	
	// Aplicar stretching
	factor := 255.0 / float64(maxLum-minLum)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			
			newR := uint8(math.Min(255, float64(uint8(r>>8)-minLum)*factor))
			newG := uint8(math.Min(255, float64(uint8(g>>8)-minLum)*factor))
			newB := uint8(math.Min(255, float64(uint8(b>>8)-minLum)*factor))
			
			enhanced.Set(x, y, color.RGBA{
				R: newR,
				G: newG,
				B: newB,
				A: uint8(a >> 8),
			})
		}
	}
	
	return enhanced
}

// binarize convierte imagen a blanco y negro
func (p *Preprocessor) binarize(img image.Image, threshold uint8) image.Image {
	bounds := img.Bounds()
	binary := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			
			// Convertir a escala de grises
			gray := uint8((0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 256)
			
			// Aplicar threshold
			if gray > threshold {
				binary.Set(x, y, color.Gray{Y: 255})
			} else {
				binary.Set(x, y, color.Gray{Y: 0})
			}
		}
	}
	
	return binary
}

// detectSkewAngle detecta el ángulo de rotación (implementación simplificada)
// En producción, usar algoritmos más avanzados como Hough Transform
func (p *Preprocessor) detectSkewAngle(img image.Image) float64 {
	// Implementación placeholder - en producción usar algoritmos avanzados
	// Por ahora retornar 0 (sin rotación detectada)
	// TODO: Implementar Hough Line Transform o similar
	return 0.0
}

// rotate rota la imagen por un ángulo dado
func (p *Preprocessor) rotate(img image.Image, angle float64) image.Image {
	// Implementación simplificada - en producción usar bibliotecas especializadas
	// como github.com/disintegration/imaging
	// Por ahora retornar imagen original
	// TODO: Implementar rotación con interpolación bilineal
	p.logger.Debug("Rotation not implemented, returning original image")
	return img
}

// median calcula la mediana de un slice de uint32
func median(values []uint32) uint32 {
	if len(values) == 0 {
		return 0
	}
	
	// Para 9 valores (3x3), tomar el 5to valor cuando está ordenado
	// Implementación simplificada: retornar valor medio
	sum := uint64(0)
	for _, v := range values {
		sum += uint64(v)
	}
	return uint32(sum / uint64(len(values)))
}
