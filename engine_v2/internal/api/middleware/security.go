package middleware

import (
	"strings"
	"time"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// SecurityHeaders aplica headers de seguridad HTTP estándar y configurables.
type SecurityHeaders struct {
	logger *logger.Logger
	config *SecurityConfig
}

// SecurityConfig define las políticas de headers de seguridad HTTP.
type SecurityConfig struct {
	AllowedOrigins         []string // Orígenes permitidos para CORS
	AllowedMethods         []string // Métodos HTTP permitidos para CORS
	AllowedHeaders         []string // Headers permitidos para CORS
	AllowCredentials       bool     // Permitir credenciales en CORS
	MaxAge                 int      // Max-Age para preflight CORS
	CSPDirectives          map[string]string // Directivas CSP
	HSTSMaxAge             int      // max-age para HSTS
	HSTSIncludeSubDomains  bool     // Incluir subdominios en HSTS
	HSTSPreload            bool     // Permitir preload en HSTS
	FrameOptions           string   // X-Frame-Options
	ContentTypeNoSniff     bool     // X-Content-Type-Options
	XSSProtection          bool     // X-XSS-Protection
	ReferrerPolicy         string   // Referrer-Policy
	PermissionsPolicy      string   // Permissions-Policy
}

// DefaultSecurityConfig retorna una configuración segura por defecto.
func DefaultSecurityConfig() *SecurityConfig {
       return &SecurityConfig{
	       AllowedOrigins: []string{
		       "https://tucentropdf.com",
		       "https://www.tucentropdf.com",
		       "https://app.tucentropdf.com",
	       },
	       AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	       AllowedHeaders: []string{
		       "Content-Type",
		       "Authorization",
		       "X-Request-ID",
		       "X-API-Key",
	       },
	       AllowCredentials: true,
	       MaxAge:           3600,
	       CSPDirectives: map[string]string{
		       "default-src":  "'self'",
		       "script-src":   "'self' 'unsafe-inline'", // TODO: Remove unsafe-inline con nonces
		       "style-src":    "'self' 'unsafe-inline'",
		       "img-src":      "'self' data: https:",
		       "font-src":     "'self' data:",
		       "connect-src":  "'self' https://api.openai.com",
		       "frame-src":    "'none'",
		       "object-src":   "'none'",
		       "base-uri":     "'self'",
		       "form-action":  "'self'",
		       "upgrade-insecure-requests": "",
	       },
	       HSTSMaxAge:            31536000,
	       HSTSIncludeSubDomains: true,
	       HSTSPreload:           true,
	       FrameOptions:          "DENY",
	       ContentTypeNoSniff:    true,
	       XSSProtection:         true,
	       ReferrerPolicy:        "strict-origin-when-cross-origin",
	       PermissionsPolicy:     "geolocation=(), microphone=(), camera=(), payment=()",
       }
}

// NewSecurityHeaders crea el middleware de security headers.
func NewSecurityHeaders(log *logger.Logger, config *SecurityConfig) *SecurityHeaders {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	return &SecurityHeaders{
		logger: log,
		config: config,
	}
}

// Handler retorna el handler middleware para Fiber.
func (sh *SecurityHeaders) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// CORS Headers
		sh.setCORSHeaders(c)

		// Security Headers
		sh.setSecurityHeaders(c)

		// Handle preflight requests
		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

// setCORSHeaders configura los headers CORS.
func (sh *SecurityHeaders) setCORSHeaders(c *fiber.Ctx) {
	origin := c.Get("Origin")

	// Validar origin contra whitelist
	if sh.isAllowedOrigin(origin) {
		c.Set("Access-Control-Allow-Origin", origin)
	}

	if sh.config.AllowCredentials {
		c.Set("Access-Control-Allow-Credentials", "true")
	}

	if len(sh.config.AllowedMethods) > 0 {
		c.Set("Access-Control-Allow-Methods", strings.Join(sh.config.AllowedMethods, ", "))
	}

	if len(sh.config.AllowedHeaders) > 0 {
		c.Set("Access-Control-Allow-Headers", strings.Join(sh.config.AllowedHeaders, ", "))
	}

	if sh.config.MaxAge > 0 {
		c.Set("Access-Control-Max-Age", strconv.Itoa(sh.config.MaxAge))
	}

	// Expose custom headers
	c.Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")
}

// setSecurityHeaders configura los headers de seguridad HTTP.
func (sh *SecurityHeaders) setSecurityHeaders(c *fiber.Ctx) {
	// Content-Security-Policy
	if len(sh.config.CSPDirectives) > 0 {
		csp := sh.buildCSPHeader()
		c.Set("Content-Security-Policy", csp)
	}

	// Strict-Transport-Security (solo HTTPS)
	if c.Protocol() == "https" {
		hsts := sh.buildHSTSHeader()
		c.Set("Strict-Transport-Security", hsts)
	}

	// X-Frame-Options
	if sh.config.FrameOptions != "" {
		c.Set("X-Frame-Options", sh.config.FrameOptions)
	}

	// X-Content-Type-Options
	if sh.config.ContentTypeNoSniff {
		c.Set("X-Content-Type-Options", "nosniff")
	}

	// X-XSS-Protection (legacy, pero bueno tener)
	if sh.config.XSSProtection {
		c.Set("X-XSS-Protection", "1; mode=block")
	}

	// Referrer-Policy
	if sh.config.ReferrerPolicy != "" {
		c.Set("Referrer-Policy", sh.config.ReferrerPolicy)
	}

	// Permissions-Policy
	if sh.config.PermissionsPolicy != "" {
		c.Set("Permissions-Policy", sh.config.PermissionsPolicy)
	}

	// X-Permitted-Cross-Domain-Policies
	c.Set("X-Permitted-Cross-Domain-Policies", "none")

	// Cache-Control para endpoints API
	if strings.HasPrefix(c.Path(), "/api/") {
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
	}

	// Server header (ocultar versión)
	c.Set("Server", "TuCentroPDF")

	// X-DNS-Prefetch-Control
	c.Set("X-DNS-Prefetch-Control", "off")
}

// isAllowedOrigin verifica si el origin está en la whitelist de CORS.
func (sh *SecurityHeaders) isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}

	for _, allowed := range sh.config.AllowedOrigins {
		if allowed == "*" {
			return true
		}
		if allowed == origin {
			return true
		}
		// Permitir subdominios si empieza con *.
		if strings.HasPrefix(allowed, "*.") {
			domain := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}

	return false
}

// buildCSPHeader construye el valor del header CSP.
func (sh *SecurityHeaders) buildCSPHeader() string {
	var directives []string

	for key, value := range sh.config.CSPDirectives {
		if value == "" {
			directives = append(directives, key)
		} else {
			directives = append(directives, key+" "+value)
		}
	}

	return strings.Join(directives, "; ")
}

// buildHSTSHeader construye el valor del header HSTS.
func (sh *SecurityHeaders) buildHSTSHeader() string {
		hsts := "max-age=" + strconv.Itoa(sh.config.HSTSMaxAge)

	if sh.config.HSTSIncludeSubDomains {
		hsts += "; includeSubDomains"
	}

	if sh.config.HSTSPreload {
		hsts += "; preload"
	}

	return hsts
}

// RateLimitHeaders añade headers informativos de rate limiting.
func RateLimitHeaders(c *fiber.Ctx, limit, remaining, resetTime int64) {
	c.Set("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
	c.Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	c.Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))

	// Retry-After si está rate limited
	if remaining <= 0 {
		retryAfter := resetTime - time.Now().Unix()
		if retryAfter > 0 {
			c.Set("Retry-After", strconv.FormatInt(retryAfter, 10))
		}
	}
}

// RequestIDMiddleware añade un X-Request-ID único a cada request.
func RequestIDMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Usar request ID del cliente si existe
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			// Generar nuevo request ID
			requestID = generateRequestID()
		}

		// Almacenar en locals para uso posterior
		c.Locals("request_id", requestID)

		// Añadir a response headers
		c.Set("X-Request-ID", requestID)

		return c.Next()
	}
}

// generateRequestID genera un ID único para el request.
func generateRequestID() string {
	// Formato: timestamp + random
	// Usar UUID para request ID
	return uuid.NewString()
}

// SecurityAuditMiddleware registra eventos de seguridad sospechosos.
type SecurityAuditMiddleware struct {
	logger *logger.Logger
}

// NewSecurityAuditMiddleware crea el middleware de auditoría de seguridad.
func NewSecurityAuditMiddleware(log *logger.Logger) *SecurityAuditMiddleware {
	return &SecurityAuditMiddleware{
		logger: log,
	}
}

// Handler registra eventos sospechosos en los requests.
func (sam *SecurityAuditMiddleware) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Detectar patrones sospechosos
		sam.detectSQLInjection(c)
		sam.detectXSS(c)
		sam.detectPathTraversal(c)
		sam.detectLargePayload(c)

		return c.Next()
	}
}

// detectSQLInjection detecta intentos de SQL injection en query o body.
func (sam *SecurityAuditMiddleware) detectSQLInjection(c *fiber.Ctx) {
	suspiciousPatterns := []string{
		"' OR '1'='1",
		"1=1",
		"UNION SELECT",
		"DROP TABLE",
		"INSERT INTO",
		"--",
		"/*",
		"xp_",
	}

	query := c.Request().URI().QueryString()
	body := string(c.Body())

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(strings.ToUpper(string(query)), strings.ToUpper(pattern)) ||
			strings.Contains(strings.ToUpper(body), strings.ToUpper(pattern)) {
			sam.logger.Warn("SQL injection attempt detected",
				"ip", c.IP(),
				"path", c.Path(),
				"pattern", pattern,
				"user_agent", c.Get("User-Agent"),
			)
			break
		}
	}
}

// detectXSS detecta intentos de XSS en query o body.
func (sam *SecurityAuditMiddleware) detectXSS(c *fiber.Ctx) {
	suspiciousPatterns := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"eval(",
		"alert(",
	}

	query := c.Request().URI().QueryString()
	body := string(c.Body())

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(strings.ToLower(string(query)), strings.ToLower(pattern)) ||
			strings.Contains(strings.ToLower(body), strings.ToLower(pattern)) {
			sam.logger.Warn("XSS attempt detected",
				"ip", c.IP(),
				"path", c.Path(),
				"pattern", pattern,
				"user_agent", c.Get("User-Agent"),
			)
			break
		}
	}
}

// detectPathTraversal detecta intentos de path traversal en path o query.
func (sam *SecurityAuditMiddleware) detectPathTraversal(c *fiber.Ctx) {
	suspiciousPatterns := []string{
		"../",
		"..\\",
		"%2e%2e",
		"....//",
	}

	path := c.Path()
	query := string(c.Request().URI().QueryString())

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(strings.ToLower(path), strings.ToLower(pattern)) ||
			strings.Contains(strings.ToLower(query), strings.ToLower(pattern)) {
			sam.logger.Warn("Path traversal attempt detected",
				"ip", c.IP(),
				"path", c.Path(),
				"pattern", pattern,
				"user_agent", c.Get("User-Agent"),
			)
			break
		}
	}
}

// detectLargePayload detecta payloads sospechosamente grandes (no multipart).
func (sam *SecurityAuditMiddleware) detectLargePayload(c *fiber.Ctx) {
	// Límite sospechoso: 100MB sin ser multipart
	const maxSuspiciousSize = 100 * 1024 * 1024

	contentLength := c.Request().Header.ContentLength()
	contentType := c.Get("Content-Type")

	// Ignorar multipart (archivos legítimos)
	if strings.Contains(contentType, "multipart/form-data") {
		return
	}

	if contentLength > maxSuspiciousSize {
		sam.logger.Warn("Suspicious large payload detected",
			"ip", c.IP(),
			"path", c.Path(),
			"content_length", contentLength,
			"content_type", contentType,
			"user_agent", c.Get("User-Agent"),
		)
	}
}
