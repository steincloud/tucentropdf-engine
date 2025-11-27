package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ValidateCaptcha valida un token de reCAPTCHA v2/v3
// @Summary Validar reCAPTCHA
// @Description Valida un token de Google reCAPTCHA
// @Tags security
// @Accept json
// @Produce json
// @Param request body CaptchaRequest true "Token de reCAPTCHA"
// @Success 200 {object} CaptchaResponse
// @Failure 400 {object} APIResponse
// @Router /api/v1/captcha/validate [post]
func (h *Handlers) ValidateCaptcha(c *fiber.Ctx) error {
	h.logger.Info("🔒 reCAPTCHA validation requested", "ip", c.IP())

	// Parse request
	var req CaptchaRequest
	if err := c.BodyParser(&req); err != nil {
		return h.ErrorResponse(c, "INVALID_REQUEST", "Invalid request body", err.Error(), 400)
	}

	// Validar token requerido
	if req.Token == "" {
		return h.ErrorResponse(c, "MISSING_TOKEN", "reCAPTCHA token is required", "", 400)
	}

	// Obtener secret key desde config
	secretKey := h.config.Captcha.SecretKey
	if secretKey == "" {
		h.logger.Error("reCAPTCHA secret key not configured")
		return h.ErrorResponse(c, "CONFIG_ERROR", "reCAPTCHA not configured", "", 500)
	}

	// Validar con Google reCAPTCHA API
	result, err := h.verifyCaptchaToken(req.Token, c.IP(), secretKey)
	if err != nil {
		h.logger.Error("reCAPTCHA verification failed", "error", err)
		return h.ErrorResponse(c, "VERIFICATION_FAILED", "Failed to verify reCAPTCHA", err.Error(), 500)
	}

	h.logger.Info("✅ reCAPTCHA validation completed",
		"success", result.Success,
		"score", result.Score,
		"action", result.Action,
	)

	return h.SuccessResponse(c, result)
}

// verifyCaptchaToken verifica token con Google reCAPTCHA API
func (h *Handlers) verifyCaptchaToken(token, remoteIP, secretKey string) (*CaptchaResponse, error) {
	// Construir request a Google
	verifyURL := "https://www.google.com/recaptcha/api/siteverify"
	
	data := url.Values{}
	data.Set("secret", secretKey)
	data.Set("response", token)
	data.Set("remoteip", remoteIP)

	// Enviar POST request
	resp, err := http.PostForm(verifyURL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to contact reCAPTCHA API: %w", err)
	}
	defer resp.Body.Close()

	// Leer respuesta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read reCAPTCHA response: %w", err)
	}

	// Parse JSON response
	var result struct {
		Success     bool      `json:"success"`
		Score       float64   `json:"score"`        // Para reCAPTCHA v3
		Action      string    `json:"action"`       // Para reCAPTCHA v3
		ChallengeTS time.Time `json:"challenge_ts"`
		Hostname    string    `json:"hostname"`
		ErrorCodes  []string  `json:"error-codes"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse reCAPTCHA response: %w", err)
	}

	// Verificar errores
	if len(result.ErrorCodes) > 0 {
		return &CaptchaResponse{
			Success:    false,
			ErrorCodes: result.ErrorCodes,
		}, nil
	}

	return &CaptchaResponse{
		Success:     result.Success,
		Score:       result.Score,
		Action:      result.Action,
		ChallengeTS: result.ChallengeTS,
		Hostname:    result.Hostname,
	}, nil
}

// CaptchaRequest estructura de request
type CaptchaRequest struct {
	Token  string `json:"token" binding:"required"`
	Action string `json:"action,omitempty"` // Para v3
}

// CaptchaResponse estructura de respuesta
type CaptchaResponse struct {
	Success     bool      `json:"success"`
	Score       float64   `json:"score,omitempty"`        // reCAPTCHA v3: 0.0 - 1.0
	Action      string    `json:"action,omitempty"`       // reCAPTCHA v3: action name
	ChallengeTS time.Time `json:"challenge_ts,omitempty"`
	Hostname    string    `json:"hostname,omitempty"`
	ErrorCodes  []string  `json:"error_codes,omitempty"`
	Message     string    `json:"message,omitempty"`
}
