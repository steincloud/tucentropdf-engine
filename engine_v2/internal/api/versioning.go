package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// VersionRouter gestiona el routing por versión de API
type VersionRouter struct {
	logger      *logger.Logger
	defaultVersion string
	versions    map[string]bool
	deprecations map[string]time.Time
}

// NewVersionRouter crea un nuevo router de versiones
func NewVersionRouter(log *logger.Logger) *VersionRouter {
	return &VersionRouter{
		logger:      log,
		defaultVersion: "v2",
		versions: map[string]bool{
			"v1": true, // Deprecated
			"v2": true, // Current
		},
		deprecations: map[string]time.Time{
			"v1": time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), // 6 meses
		},
	}
}

// VersionMiddleware extrae versión del request
func (vr *VersionRouter) VersionMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		version := vr.extractVersion(c)

		// Validar versión
		if !vr.isVersionSupported(version) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    "UNSUPPORTED_API_VERSION",
					"message": fmt.Sprintf("API version '%s' is not supported", version),
					"supported_versions": []string{"v1", "v2"},
				},
			})
		}

		// Guardar versión en locals
		c.Locals("api_version", version)

		// Añadir deprecation headers si aplica
		if deprecationDate, isDeprecated := vr.deprecations[version]; isDeprecated {
			c.Set("Deprecation", "true")
			c.Set("Sunset", deprecationDate.Format(time.RFC1123))
			c.Set("Link", `</api/v2>; rel="alternate"`)
			
			// Warning header (RFC 7234)
			warningMsg := fmt.Sprintf(
				`299 - "API %s is deprecated and will be sunset on %s. Please migrate to v2."`,
				version,
				deprecationDate.Format("2006-01-02"),
			)
			c.Set("Warning", warningMsg)
		}

		// Añadir versión a response headers
		c.Set("X-API-Version", version)

		return c.Next()
	}
}

// extractVersion extrae la versión del request
func (vr *VersionRouter) extractVersion(c *fiber.Ctx) string {
	// 1. URL path: /api/v2/...
	path := c.Path()
	if strings.HasPrefix(path, "/api/v") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	// 2. Header: X-API-Version
	if version := c.Get("X-API-Version"); version != "" {
		return version
	}

	// 3. Query param: ?api_version=v2
	if version := c.Query("api_version"); version != "" {
		return version
	}

	// 4. Accept header: application/vnd.tucentropdf.v2+json
	if accept := c.Get("Accept"); accept != "" {
		if strings.Contains(accept, "vnd.tucentropdf.v") {
			start := strings.Index(accept, ".v")
			if start != -1 {
				versionPart := accept[start+2:]
				end := strings.IndexAny(versionPart, "+;,")
				if end != -1 {
					return "v" + versionPart[:end]
				}
			}
		}
	}

	// Default
	return vr.defaultVersion
}

// isVersionSupported verifica si una versión está soportada
func (vr *VersionRouter) isVersionSupported(version string) bool {
	supported, exists := vr.versions[version]
	return exists && supported
}

// GetVersion retorna la versión del request actual
func GetVersion(c *fiber.Ctx) string {
	if version, ok := c.Locals("api_version").(string); ok {
		return version
	}
	return "v2"
}

// V1Router crea subrouter para v1 (legacy)
func V1Router(app *fiber.App, log *logger.Logger) fiber.Router {
	v1 := app.Group("/api/v1")

	// Middleware de deprecation warning
	v1.Use(func(c *fiber.Ctx) error {
		log.Warn("V1 API access",
			"path", c.Path(),
			"ip", c.IP(),
			"user_agent", c.Get("User-Agent"),
		)
		return c.Next()
	})

	return v1
}

// V2Router crea subrouter para v2 (actual)
func V2Router(app *fiber.App) fiber.Router {
	return app.Group("/api/v2")
}

// VersionedResponse estructura de respuesta con versión
type VersionedResponse struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data,omitempty"`
	Error      interface{} `json:"error,omitempty"`
	Metadata   *Metadata   `json:"metadata"`
}

// Metadata metadatos de la respuesta
type Metadata struct {
	Version      string    `json:"version"`
	Timestamp    time.Time `json:"timestamp"`
	RequestID    string    `json:"request_id,omitempty"`
	Deprecated   bool      `json:"deprecated,omitempty"`
	SunsetDate   string    `json:"sunset_date,omitempty"`
	AlternateURL string    `json:"alternate_url,omitempty"`
}

// RespondV2 responde con formato v2
func RespondV2(c *fiber.Ctx, data interface{}) error {
	response := VersionedResponse{
		Success: true,
		Data:    data,
		Metadata: &Metadata{
			Version:   "v2",
			Timestamp: time.Now(),
			RequestID: c.Locals("request_id").(string),
		},
	}

	return c.JSON(response)
}

// RespondV2Error responde con error en formato v2
func RespondV2Error(c *fiber.Ctx, code, message string, statusCode int) error {
	response := VersionedResponse{
		Success: false,
		Error: fiber.Map{
			"code":    code,
			"message": message,
		},
		Metadata: &Metadata{
			Version:   "v2",
			Timestamp: time.Now(),
			RequestID: c.Locals("request_id").(string),
		},
	}

	return c.Status(statusCode).JSON(response)
}

// RespondV1 responde con formato v1 (legacy)
func RespondV1(c *fiber.Ctx, data interface{}) error {
	// Formato v1 simple (sin metadata)
	return c.JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// RespondV1Error responde con error en formato v1 (legacy)
func RespondV1Error(c *fiber.Ctx, message string, statusCode int) error {
	return c.Status(statusCode).JSON(fiber.Map{
		"success": false,
		"error":   message,
	})
}

// TranslateV1ToV2 traduce request v1 a v2
func TranslateV1ToV2(v1Data map[string]interface{}) map[string]interface{} {
	v2Data := make(map[string]interface{})

	// Mapear campos que cambiaron entre v1 y v2
	for key, value := range v1Data {
		switch key {
		case "file_id":
			v2Data["file_id"] = value
		case "operation":
			v2Data["tool"] = value // v1: "operation", v2: "tool"
		case "options":
			v2Data["params"] = value // v1: "options", v2: "params"
		case "callback_url":
			v2Data["webhook_url"] = value
		default:
			v2Data[key] = value
		}
	}

	return v2Data
}

// TranslateV2ToV1 traduce response v2 a v1 (para backward compatibility)
func TranslateV2ToV1(v2Data map[string]interface{}) map[string]interface{} {
	v1Data := make(map[string]interface{})

	for key, value := range v2Data {
		switch key {
		case "tool":
			v1Data["operation"] = value
		case "params":
			v1Data["options"] = value
		case "webhook_url":
			v1Data["callback_url"] = value
		case "metadata":
			// Omitir metadata en v1
			continue
		default:
			v1Data[key] = value
		}
	}

	return v1Data
}

// ContentNegotiation maneja content negotiation
func ContentNegotiation() fiber.Handler {
	return func(c *fiber.Ctx) error {
		accept := c.Get("Accept")

		// Determinar formato de respuesta
		if strings.Contains(accept, "application/json") {
			c.Locals("response_format", "json")
		} else if strings.Contains(accept, "application/xml") {
			c.Locals("response_format", "xml")
		} else if strings.Contains(accept, "text/plain") {
			c.Locals("response_format", "text")
		} else {
			// Default JSON
			c.Locals("response_format", "json")
		}

		return c.Next()
	}
}

// CacheControlMiddleware añade headers de cache según versión
func CacheControlMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		version := GetVersion(c)

		// v2 es más estricto con cache
		if version == "v2" {
			c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		} else {
			c.Set("Cache-Control", "no-cache")
		}

		return c.Next()
	}
}

// VersionInfo estructura de información de versión
type VersionInfo struct {
	Version      string    `json:"version"`
	Status       string    `json:"status"` // stable, deprecated, sunset
	ReleaseDate  string    `json:"release_date"`
	SunsetDate   string    `json:"sunset_date,omitempty"`
	Features     []string  `json:"features"`
	BreakingChanges []string `json:"breaking_changes,omitempty"`
	MigrationGuide string   `json:"migration_guide,omitempty"`
}

// GetVersionsInfo retorna información de todas las versiones
func GetVersionsInfo() []VersionInfo {
	return []VersionInfo{
		{
			Version:     "v1",
			Status:      "deprecated",
			ReleaseDate: "2024-01-15",
			SunsetDate:  "2026-06-01",
			Features: []string{
				"Basic PDF operations",
				"OCR Classic",
				"Office conversion",
			},
			BreakingChanges: []string{
				"'operation' renamed to 'tool' in v2",
				"'options' renamed to 'params' in v2",
				"Response format changed (added metadata)",
			},
			MigrationGuide: "https://docs.tucentropdf.com/migration/v1-to-v2",
		},
		{
			Version:     "v2",
			Status:      "stable",
			ReleaseDate: "2025-11-01",
			Features: []string{
				"All v1 features",
				"OCR AI (GPT-4 Vision)",
				"Batch processing",
				"Advanced rate limiting",
				"Cost tracking",
				"Enhanced security",
			},
		},
	}
}

// VersionsHandler endpoint GET /api/versions
func VersionsHandler(c *fiber.Ctx) error {
	versions := GetVersionsInfo()

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"versions":        versions,
			"default_version": "v2",
			"latest_version":  "v2",
		},
	})
}

// PaginationParams parámetros de paginación (v2)
type PaginationParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Offset   int `json:"offset"`
	Limit    int `json:"limit"`
}

// ExtractPaginationV2 extrae paginación en formato v2
func ExtractPaginationV2(c *fiber.Ctx) PaginationParams {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))

	// Validar límites
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Max 100 items
	}

	offset := (page - 1) * pageSize

	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
		Limit:    pageSize,
	}
}

// PaginatedResponse respuesta paginada (v2)
type PaginatedResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
	Metadata *Metadata `json:"metadata"`
}

// PaginationMeta metadatos de paginación
type PaginationMeta struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	TotalItems int  `json:"total_items"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

// RespondPaginatedV2 responde con datos paginados
func RespondPaginatedV2(c *fiber.Ctx, data interface{}, pagination PaginationParams, totalItems int) error {
	totalPages := (totalItems + pagination.PageSize - 1) / pagination.PageSize

	response := PaginatedResponse{
		Success: true,
		Data:    data,
		Pagination: PaginationMeta{
			Page:       pagination.Page,
			PageSize:   pagination.PageSize,
			TotalItems: totalItems,
			TotalPages: totalPages,
			HasNext:    pagination.Page < totalPages,
			HasPrev:    pagination.Page > 1,
		},
		Metadata: &Metadata{
			Version:   "v2",
			Timestamp: time.Now(),
			RequestID: c.Locals("request_id").(string),
		},
	}

	return c.JSON(response)
}
