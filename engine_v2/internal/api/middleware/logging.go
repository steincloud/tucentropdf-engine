package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RequestLogger middleware para logging de requests
func RequestLogger(log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Procesar request
		err := c.Next()

		// Calcular duración
		duration := time.Since(start)
		status := c.Response().StatusCode()

		// Preparar campos de log
		fields := map[string]interface{}{
			"method":       c.Method(),
			"path":         c.Path(),
			"status":       status,
			"duration_ms":  duration.Milliseconds(),
			"client_ip":    c.IP(),
			"user_agent":   c.Get("User-Agent"),
			"request_id":   c.Get("X-Request-ID"),
			"content_type": c.Get("Content-Type"),
			"bytes_sent":   len(c.Response().Body()),
		}

		// Añadir query params si existen
		if c.Request().URI().QueryString() != nil {
			fields["query"] = string(c.Request().URI().QueryString())
		}

		// Logging basado en status
		logger := log.WithFields(fields)
		
		switch {
		case status >= 500:
			logger.Error("❌ Request failed")
		case status >= 400:
			logger.Warn("⚠️ Request error")
		case status >= 300:
			logger.Info("➡️ Request redirect")
		default:
			logger.Info("✅ Request success")
		}

		return err
	}
}

// ErrorHandler maneja errores de forma consistente
func ErrorHandler(log *logger.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		// Default 500 status code
		code := fiber.StatusInternalServerError
		message := "Internal Server Error"

		// Si es un error de Fiber
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		}

		// Log del error
		logFields := map[string]interface{}{
			"error":     err.Error(),
			"status":    code,
			"method":    c.Method(),
			"path":      c.Path(),
			"client_ip": c.IP(),
		}

		if code >= 500 {
			log.WithFields(logFields).Error("🛑 Server error")
		} else {
			log.WithFields(logFields).Warn("⚠️ Client error")
		}

		// Respuesta JSON estructurada
		errorResponse := fiber.Map{
			"error":     message,
			"code":      getErrorCode(code),
			"timestamp": time.Now().UTC(),
			"path":      c.Path(),
		}

		// Añadir detalles en desarrollo
		if c.App().Config().AppName == "development" {
			errorResponse["details"] = err.Error()
		}

		return c.Status(code).JSON(errorResponse)
	}
}

// getErrorCode convierte status HTTP a código de error
func getErrorCode(status int) string {
	switch status {
	case 400:
		return "BAD_REQUEST"
	case 401:
		return "UNAUTHORIZED"
	case 403:
		return "FORBIDDEN"
	case 404:
		return "NOT_FOUND"
	case 413:
		return "PAYLOAD_TOO_LARGE"
	case 415:
		return "UNSUPPORTED_MEDIA_TYPE"
	case 429:
		return "TOO_MANY_REQUESTS"
	case 500:
		return "INTERNAL_SERVER_ERROR"
	case 502:
		return "BAD_GATEWAY"
	case 503:
		return "SERVICE_UNAVAILABLE"
	case 504:
		return "GATEWAY_TIMEOUT"
	default:
		return "UNKNOWN_ERROR"
	}
}