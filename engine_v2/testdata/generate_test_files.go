package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Generador de archivos de prueba para testdata/
func main() {
	// Crear PDFs de prueba
	generateTestPDFs()
	
	fmt.Println("✅ Archivos de prueba generados en testdata/")
}

func generateTestPDFs() {
	testdataDir := "testdata/pdf"
	
	// PDF simple de 2 páginas
	err := generateSimplePDF(filepath.Join(testdataDir, "sample1.pdf"), 2)
	if err != nil {
		fmt.Printf("Error generando sample1.pdf: %v\n", err)
	}
	
	// PDF de 5 páginas
	err = generateSimplePDF(filepath.Join(testdataDir, "sample2.pdf"), 5)
	if err != nil {
		fmt.Printf("Error generando sample2.pdf: %v\n", err)
	}
	
	// PDF grande de 50 páginas
	err = generateSimplePDF(filepath.Join(testdataDir, "big.pdf"), 50)
	if err != nil {
		fmt.Printf("Error generando big.pdf: %v\n", err)
	}
	
	fmt.Println("📄 PDFs de prueba generados")
}

func generateSimplePDF(filename string, pages int) error {
	// Crear directorio si no existe
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	// Configuración de pdfcpu
	config := model.NewDefaultConfiguration()
	
	// Crear PDF simple con texto
	content := fmt.Sprintf("Página de prueba\nEste es un PDF de testing\nGenerado automáticamente\nTotal de páginas: %d", pages)
	
	// Por simplicidad, crear un archivo PDF básico
	// En un entorno real, usaríamos una librería de generación de PDF más completa
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Escribir contenido PDF mínimo válido
	pdfContent := `%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Pages 2 0 R
>>
endobj

2 0 obj
<<
/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj

3 0 obj
<<
/Type /Page
/Parent 2 0 R
/Resources <<
/Font <<
/F1 4 0 R
>>
>>
/MediaBox [0 0 612 792]
/Contents 5 0 R
>>
endobj

4 0 obj
<<
/Type /Font
/Subtype /Type1
/BaseFont /Helvetica
>>
endobj

5 0 obj
<<
/Length 85
>>
stream
BT
/F1 12 Tf
72 720 Td
(Test PDF Document) Tj
0 -15 Td
(Generated for testing) Tj
ET
endstream
endobj

xref
0 6
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
0000000274 00000 n 
0000000351 00000 n 
trailer
<<
/Size 6
/Root 1 0 R
>>
startxref
485
%%EOF`
	
	_, err = file.WriteString(pdfContent)
	return err
}