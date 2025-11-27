package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	generateOCRTestImages()
	fmt.Println("✅ Imágenes de prueba OCR generadas")
}

func generateOCRTestImages() {
	// Imagen limpia con texto claro
	generateTextImage("testdata/ocr/classic/clean.png", "HELLO WORLD\nThis is clean text for OCR testing\n2025", false, false)
	
	// Imagen para AI OCR - Factura simulada
	generateInvoiceImage("testdata/ocr/ai/invoice.png")
	
	// Imagen para AI OCR - ID simulado
	generateIDImage("testdata/ocr/ai/id_card.png")
}

func generateTextImage(filename, text string, blur, tilt bool) {
	// Crear directorio si no existe
	dir := filepath.Dir(filename)
	os.MkdirAll(dir, 0755)
	
	// Crear imagen simple con texto
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	
	// Fondo blanco
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	
	// Simular texto negro (pixels básicos)
	// Texto "HELLO WORLD" en coordenadas específicas
	drawSimpleText(img, 50, 50, text)
	
	// Guardar imagen
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creando %s: %v\n", filename, err)
		return
	}
	defer file.Close()
	
	png.Encode(file, img)
}

func generateInvoiceImage(filename string) {
	dir := filepath.Dir(filename)
	os.MkdirAll(dir, 0755)
	
	img := image.NewRGBA(image.Rect(0, 0, 600, 800))
	
	// Fondo blanco
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	
	// Simular estructura de factura
	drawSimpleText(img, 50, 50, "FACTURA\nEmpresa XYZ\nDireccion: Calle 123\nTotal: $1,250.00\nFecha: 14/11/2025")
	
	file, _ := os.Create(filename)
	defer file.Close()
	png.Encode(file, img)
}

func generateIDImage(filename string) {
	dir := filepath.Dir(filename)
	os.MkdirAll(dir, 0755)
	
	img := image.NewRGBA(image.Rect(0, 0, 500, 300))
	
	// Fondo blanco
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	
	// Simular ID
	drawSimpleText(img, 50, 50, "ID CARD\nName: Juan Perez\nDNI: 12345678\nBirth: 01/01/1990")
	
	file, _ := os.Create(filename)
	defer file.Close()
	png.Encode(file, img)
}

func drawSimpleText(img *image.RGBA, x, y int, text string) {
	// Simular texto básico con pixeles negros
	// Esta es una implementación muy simple para testing
	black := color.RGBA{0, 0, 0, 255}
	
	// Crear patrones básicos de letras (muy simplificado)
	for i, char := range text {
		if char == '\n' {
			y += 20
			x = 50
			continue
		}
		
		// Dibujar caracteres como rectángulos pequeños (simulación)
		for dx := 0; dx < 8; dx++ {
			for dy := 0; dy < 12; dy++ {
				if x+dx < img.Bounds().Max.X && y+dy < img.Bounds().Max.Y {
					if (dx == 0 || dx == 7 || dy == 0 || dy == 11) && char != ' ' {
						img.Set(x+dx, y+dy, black)
					}
				}
			}
		}
		x += 10
	}
}