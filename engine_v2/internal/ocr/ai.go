package ocr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// AIService interface para OCR con IA
type AIService interface {
	ExtractText(imagePath string) (*AIResult, error)
	ExtractStructuredData(imagePath string, options *ExtractOptions) (*AIResult, error)
	IsAvailable() bool
	GetUsageLimits(plan string) int
}

// AIResult resultado de OCR con IA
type AIResult struct {
	Text           string                 `json:"text"`
	StructuredData map[string]interface{} `json:"structured_data,omitempty"`
	Language       string                 `json:"language"`
	Confidence     float64                `json:"confidence"`
	Engine         string                 `json:"engine"`
	Model          string                 `json:"model"`
	Duration       time.Duration          `json:"duration_ms"`
	TokensUsed     int                    `json:"tokens_used"`
	Cost           float64                `json:"cost_usd"`
	Metadata       map[string]string      `json:"metadata"`
}

// ExtractOptions opciones para extracción estructurada
type ExtractOptions struct {
	ExtractTables bool `json:"extract_tables"`
	ExtractForms  bool `json:"extract_forms"`
	Language      string `json:"language,omitempty"`
	Format        string `json:"format"` // "text", "json", "markdown"
}

// OpenAIService implementación de OCR con OpenAI Vision
type OpenAIService struct {
	config    *config.Config
	apiKey    string
	model     string
	baseURL   string
	client    *http.Client
	logger    *logger.Logger
	reqCount  int
	totalCost float64
}

// NewOpenAIService crear nuevo servicio OpenAI
func NewOpenAIService(cfg *config.Config, log *logger.Logger) *OpenAIService {
	return &OpenAIService{
		config:  cfg,
		apiKey:  cfg.AI.APIKey,
		model:   "gpt-4o-mini", // Usar modelo especificado
		baseURL: "https://api.openai.com/v1",
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: log,
	}
}

// ValidatePlanAILimits validar límites de IA por plan
func (s *OpenAIService) ValidatePlanAILimits(plan string, pagesCount int) error {
	limits := s.GetUsageLimits(plan)
	
	if limits == 0 {
		return fmt.Errorf("plan %s no tiene acceso a OCR con IA", plan)
	}
	
	if pagesCount > limits {
		return fmt.Errorf("el documento tiene %d páginas, pero el plan %s solo permite %d páginas con IA", 
			pagesCount, plan, limits)
	}
	
	return nil
}

// GetUsageLimits obtener límites de uso por plan
func (s *OpenAIService) GetUsageLimits(plan string) int {
	switch strings.ToLower(plan) {
	case "free":
		return 0 // Free plan no tiene acceso a IA
	case "premium":
		return 3 // Premium plan: 3 páginas
	case "pro":
		return 20 // Pro plan: 20 páginas
	default:
		return 0
	}
}

// NewAIService crea una instancia del servicio OCR IA
func NewAIService(cfg *config.Config, log *logger.Logger) AIService {
	if !cfg.AI.Enabled || cfg.AI.APIKey == "" {
		return &DisabledAIService{logger: log}
	}

	return &OpenAIService{
		config:  cfg,
		logger:  log,
		apiKey:  cfg.AI.APIKey,
		model:   cfg.AI.Model,
		baseURL: "https://api.openai.com/v1",
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// OpenAI Implementation
func (s *OpenAIService) ExtractText(imagePath string) (*AIResult, error) {
	options := &ExtractOptions{
		ExtractTables: false,
		ExtractForms:  false,
		Format:        "text",
	}
	return s.ExtractStructuredData(imagePath, options)
}

func (s *OpenAIService) ExtractStructuredData(imagePath string, options *ExtractOptions) (*AIResult, error) {
	startTime := time.Now()
	
	s.logger.Info("🤖 Starting OpenAI Vision OCR",
		"image", filepath.Base(imagePath),
		"model", s.model,
		"extract_tables", options.ExtractTables,
		"extract_forms", options.ExtractForms,
	)

	// Validar imagen
	if err := validateImageFile(imagePath); err != nil {
		return nil, fmt.Errorf("invalid image: %w", err)
	}

	// Convertir imagen a base64
	imageData, err := s.encodeImageToBase64(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}

	// Preparar prompt según opciones
	prompt := s.buildPrompt(options)

	// Construir request para OpenAI
	requestBody := map[string]interface{}{
		"model": s.model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": prompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imageData,
						},
					},
				},
			},
		},
		"max_tokens": 4000,
		"temperature": 0.1, // Baja temperatura para mayor precisión
	}

	// Llamar a OpenAI API
	response, err := s.callOpenAI(requestBody)
	if err != nil {
		s.logger.Error("❌ OpenAI Vision OCR failed",
			"image", filepath.Base(imagePath),
			"model", s.model,
			"error", err.Error(),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	duration := time.Since(startTime)

	// Procesar respuesta
	result := &AIResult{
		Text:       response.Text,
		Language:   detectLanguage(response.Text),
		Confidence: 0.95, // Alta confianza para GPT-4.1-mini
		Engine:     "openai_vision",
		Model:      s.model,
		Duration:   duration,
		TokensUsed: response.TokensUsed,
		Cost:       s.calculateCost(response.TokensUsed),
		Metadata: map[string]string{
			"input_size":      fmt.Sprintf("%d", getFileSize(imagePath)),
			"extract_tables":  fmt.Sprintf("%t", options.ExtractTables),
			"extract_forms":   fmt.Sprintf("%t", options.ExtractForms),
			"format":          options.Format,
		},
	}

	// Extraer datos estructurados si se solicita
	if options.ExtractTables || options.ExtractForms {
		result.StructuredData = s.parseStructuredData(response.Text, options)
	}

	s.logger.Info("✅ OpenAI Vision OCR completed",
		"image", filepath.Base(imagePath),
		"model", s.model,
		"text_length", len(response.Text),
		"tokens_used", response.TokensUsed,
		"cost_usd", fmt.Sprintf("%.4f", result.Cost),
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

func (s *OpenAIService) IsAvailable() bool {
	return s.config.AI.Enabled && s.apiKey != ""
}

// buildPrompt construye el prompt para OpenAI basado en las opciones
func (s *OpenAIService) buildPrompt(options *ExtractOptions) string {
	prompt := "Extract all text from this image. Be precise and maintain formatting."
	
	if options.ExtractTables {
		prompt += " If there are tables, preserve the table structure using markdown format."
	}
	
	if options.ExtractForms {
		prompt += " If there are forms, extract field names and values clearly."
	}
	
	if options.Language != "" {
		prompt += " The text is primarily in " + options.Language + "."
	}
	
	switch options.Format {
	case "json":
		prompt += " Return the result as structured JSON."
	case "markdown":
		prompt += " Return the result in markdown format."
	default:
		prompt += " Return the text as plain text."
	}
	
	return prompt
}

// parseStructuredData parsea datos estructurados del texto de respuesta
func (s *OpenAIService) parseStructuredData(text string, options *ExtractOptions) map[string]interface{} {
	result := make(map[string]interface{})
	
	if options.ExtractTables {
		// Buscar tablas en el texto (implementación básica)
		result["tables"] = []string{} // Placeholder para tablas extraídas
	}
	
	if options.ExtractForms {
		// Buscar campos de formulario (implementación básica)
		result["forms"] = map[string]string{} // Placeholder para campos de formulario
	}
	
	// Agregar el texto completo
	result["raw_text"] = text
	
	return result
}

// Helper methods
func (s *OpenAIService) encodeImageToBase64(imagePath string) (string, error) {
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return "", err
	}

	// Detectar MIME type
	mimeType := "image/jpeg"
	ext := strings.ToLower(filepath.Ext(imagePath))
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".webp":
		mimeType = "image/webp"
	}

	base64Data := base64.StdEncoding.EncodeToString(imageBytes)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data), nil
}

// buildStructuredPrompt construir prompt para extracción estructurada
func (s *OpenAIService) buildStructuredPrompt(options *ExtractOptions) string {
	prompt := "Analiza esta imagen y extrae todo el texto con la mayor precisión posible."
	
	if options.ExtractTables {
		prompt += " Si hay tablas, preserva su estructura usando formato markdown."
	}
	
	if options.ExtractForms {
		prompt += " Si hay campos de formulario, identifica claramente las etiquetas y valores."
	}
	
	switch options.Format {
	case "json":
		prompt += " Devuelve el resultado en formato JSON con el campo 'text' y cualquier dato estructurado."
	case "markdown":
		prompt += " Formatea la salida en markdown limpio y bien estructurado."
	default:
		prompt += " Devuelve texto plano limpio y bien formateado."
	}
	
	prompt += " Mantén el idioma original y preserva caracteres especiales, números y formato."
	
	if options.Language != "" && options.Language != "auto" {
		prompt += fmt.Sprintf(" El texto principal está en idioma: %s", options.Language)
	}
	
	return prompt
}

type OpenAIResponse struct {
	Text       string `json:"text"`
	TokensUsed int    `json:"tokens_used"`
}

func (s *OpenAIService) callOpenAI(requestBody map[string]interface{}) (*OpenAIResponse, error) {
	// Serializar request
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	// Crear HTTP request
	req, err := http.NewRequest("POST", s.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	// Headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// Ejecutar request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Leer respuesta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Verificar status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error: %d - %s", resp.StatusCode, string(body))
	}

	// Parsear respuesta
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	return &OpenAIResponse{
		Text:       apiResp.Choices[0].Message.Content,
		TokensUsed: apiResp.Usage.TotalTokens,
	}, nil
}

func (s *OpenAIService) calculateCost(tokens int) float64 {
	// GPT-4.1-mini pricing (aproximado)
	// Input: $0.00015 / 1K tokens
	// Output: $0.0006 / 1K tokens
	// Promedio: $0.000375 / 1K tokens
	costPer1K := 0.000375
	return float64(tokens) * costPer1K / 1000
}

// parseStructuredResponse parsear respuesta estructurada
func (s *OpenAIService) parseStructuredResponse(text string, options *ExtractOptions) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	
	// Si el formato es JSON, intentar parsear
	if options.Format == "json" {
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(text), &jsonData); err == nil {
			return jsonData, nil
		}
	}
	
	// Extraer estructuras si se solicitó
	if options.ExtractTables {
		tables := s.extractTablesFromText(text)
		if len(tables) > 0 {
			data["tables"] = tables
		}
	}
	
	if options.ExtractForms {
		forms := s.extractFormsFromText(text)
		if len(forms) > 0 {
			data["forms"] = forms
		}
	}
	
	// Si no se encontró estructura, retornar nil
	if len(data) == 0 {
		return nil, nil
	}
	
	return data, nil
}

// DisabledAIService cuando IA está deshabilitada
type DisabledAIService struct {
	logger *logger.Logger
}

func (s *DisabledAIService) ExtractText(imagePath string) (*AIResult, error) {
	return nil, fmt.Errorf("AI OCR is disabled")
}

func (s *DisabledAIService) ExtractStructuredData(imagePath string, options *ExtractOptions) (*AIResult, error) {
	return nil, fmt.Errorf("AI OCR is disabled")
}

func (s *DisabledAIService) IsAvailable() bool {
	return false
}

func (s *DisabledAIService) GetUsageLimits(plan string) int {
	return 0
}

// Utility functions
func detectLanguage(text string) string {
	// Detección básica de idioma
	text = strings.ToLower(text)
	
	spanishWords := []string{"el", "la", "de", "que", "y", "en", "un", "es", "se", "no", "te", "lo", "le", "da", "su", "por", "son", "con", "para", "como", "está", "tú", "muy", "todo", "pero", "más", "hacer", "ser", "aquí", "sobre"}
	englishWords := []string{"the", "and", "is", "in", "to", "of", "a", "that", "it", "with", "for", "as", "was", "on", "are", "you", "this", "be", "at", "or", "have", "from", "an", "they", "which", "one", "by", "word", "but", "not"}
	
	spanishCount := 0
	englishCount := 0
	
	words := strings.Fields(text)
	for _, word := range words[:min(len(words), 50)] { // Check first 50 words
		for _, sw := range spanishWords {
			if word == sw {
				spanishCount++
			}
		}
		for _, ew := range englishWords {
			if word == ew {
				englishCount++
			}
		}
	}
	
	if spanishCount > englishCount {
		return "es"
	}
	return "en"
}

func extractTables(text string) []map[string]interface{} {
	// TODO: Implementar extracción de tablas más sofisticada
	return []map[string]interface{}{}
}

func extractForms(text string) []map[string]interface{} {
	// TODO: Implementar extracción de formularios más sofisticada  
	return []map[string]interface{}{}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractTablesFromText extraer tablas del texto
func (s *OpenAIService) extractTablesFromText(text string) []map[string]interface{} {
	tables := make([]map[string]interface{}, 0)
	
	// Buscar patrones de tablas en markdown
	lines := strings.Split(text, "\n")
	var currentTable []string
	inTable := false
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Detectar líneas de tabla (contienen |)
		if strings.Contains(line, "|") && len(strings.Split(line, "|")) >= 3 {
			inTable = true
			currentTable = append(currentTable, line)
		} else if inTable {
			// Fin de tabla
			if len(currentTable) > 1 {
				table := s.parseMarkdownTable(currentTable)
				if table != nil {
					tables = append(tables, table)
				}
			}
			currentTable = []string{}
			inTable = false
		}
	}
	
	// Verificar si hay una tabla al final
	if inTable && len(currentTable) > 1 {
		table := s.parseMarkdownTable(currentTable)
		if table != nil {
			tables = append(tables, table)
		}
	}
	
	return tables
}

// extractFormsFromText extraer formularios del texto
func (s *OpenAIService) extractFormsFromText(text string) []map[string]interface{} {
	forms := make([]map[string]interface{}, 0)
	
	// Buscar patrones de formularios (campo: valor)
	lines := strings.Split(text, "\n")
	currentForm := make(map[string]string)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Buscar patrones como "Campo:", "Campo: valor", etc.
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				field := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				
				// Si el valor está vacío, puede ser que esté en la siguiente línea
				if value == "" && len(lines) > 0 {
					continue
				}
				
				currentForm[field] = value
			}
		}
	}
	
	if len(currentForm) > 0 {
		forms = append(forms, map[string]interface{}{
			"fields": currentForm,
			"type":   "extracted_form",
		})
	}
	
	return forms
}

// parseMarkdownTable parsear tabla en formato markdown
func (s *OpenAIService) parseMarkdownTable(lines []string) map[string]interface{} {
	if len(lines) < 2 {
		return nil
	}
	
	// Primera línea son los headers
	headers := s.parseTableRow(lines[0])
	if len(headers) == 0 {
		return nil
	}
	
	// Buscar línea separadora (contiene -)
	separatorIndex := -1
	for i, line := range lines {
		if strings.Contains(line, "-") && strings.Contains(line, "|") {
			separatorIndex = i
			break
		}
	}
	
	if separatorIndex == -1 || separatorIndex >= len(lines)-1 {
		return nil
	}
	
	// Parsear filas de datos
	rows := make([]map[string]string, 0)
	for i := separatorIndex + 1; i < len(lines); i++ {
		rowData := s.parseTableRow(lines[i])
		if len(rowData) == len(headers) {
			row := make(map[string]string)
			for j, header := range headers {
				row[header] = rowData[j]
			}
			rows = append(rows, row)
		}
	}
	
	return map[string]interface{}{
		"headers": headers,
		"rows":    rows,
		"type":    "markdown_table",
	}
}

// parseTableRow parsear fila de tabla
func (s *OpenAIService) parseTableRow(line string) []string {
	if !strings.Contains(line, "|") {
		return nil
	}
	
	parts := strings.Split(line, "|")
	result := make([]string, 0)
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	
	return result
}