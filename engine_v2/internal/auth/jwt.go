package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/go-redis/redis/v8"
)

// AdminClaims define las reclamaciones JWT para administradores
type AdminClaims struct {
	UserID      int64    `json:"user_id"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// JWTManager gestiona tokens JWT para administradores
type JWTManager struct {
	secretKey     []byte
	issuer        string
	audience      string
	tokenDuration time.Duration
	redisClient   *redis.Client
}

// NewJWTManager crea nueva instancia del gestor JWT
func NewJWTManager(redisClient *redis.Client) (*JWTManager, error) {
	secretKey := os.Getenv("JWT_SECRET_KEY")
	if secretKey == "" {
		return nil, errors.New("JWT_SECRET_KEY environment variable is required")
	}

	if len(secretKey) < 32 {
		return nil, errors.New("JWT_SECRET_KEY must be at least 32 characters long")
	}

	// Configuración desde variables de entorno
	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "tucentropdf-engine-v2"
	}

	audience := os.Getenv("JWT_ADMIN_AUDIENCE")
	if audience == "" {
		audience = "tucentropdf-legal-audit"
	}

	// Duración del token desde variable de entorno
	durationStr := os.Getenv("JWT_ADMIN_TOKEN_DURATION")
	duration := 8 * time.Hour // Por defecto 8 horas

	if durationStr != "" {
		if hours, err := strconv.Atoi(durationStr); err == nil && hours > 0 && hours <= 24 {
			duration = time.Duration(hours) * time.Hour
		}
	}

	return &JWTManager{
		secretKey:     []byte(secretKey),
		issuer:        issuer,
		audience:      audience,
		tokenDuration: duration,
		redisClient:   redisClient,
	}, nil
}

// GenerateAdminToken genera token JWT para administrador
func (j *JWTManager) GenerateAdminToken(userID int64, email, role string, permissions []string) (string, error) {
	now := time.Now()
	claims := AdminClaims{
		UserID:      userID,
		Email:       email,
		Role:        role,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Audience:  jwt.ClaimStrings{j.audience},
			Subject:   fmt.Sprintf("admin_%d", userID),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.tokenDuration)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        generateJTI(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secretKey)
}

// ValidateAdminToken valida token JWT y retorna claims
func (j *JWTManager) ValidateAdminToken(tokenString string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&AdminClaims{},
		func(token *jwt.Token) (interface{}, error) {
			// Verificar algoritmo de firma
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return j.secretKey, nil
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// Verificar issuer
	if claims.Issuer != j.issuer {
		return nil, errors.New("invalid token issuer")
	}

	// Verificar audience
	if len(claims.Audience) == 0 || claims.Audience[0] != j.audience {
		return nil, errors.New("invalid token audience")
	}

	// Verificar expiración
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token has expired")
	}

	// Verificar revocación
	if j.IsTokenRevoked(claims.ID) {
		return nil, errors.New("token has been revoked")
	}

	return claims, nil
}

// RefreshAdminToken renueva token si está próximo a expirar
func (j *JWTManager) RefreshAdminToken(tokenString string) (string, error) {
	claims, err := j.ValidateAdminToken(tokenString)
	if err != nil {
		return "", err
	}

	// Solo renovar si el token expira en menos de 1 hora
	if claims.ExpiresAt != nil && time.Until(claims.ExpiresAt.Time) > time.Hour {
		return tokenString, nil // No necesita renovación
	}

	// Generar nuevo token con los mismos permisos
	return j.GenerateAdminToken(claims.UserID, claims.Email, claims.Role, claims.Permissions)
}

// HasPermission verifica si el token tiene permiso específico
func (j *JWTManager) HasPermission(claims *AdminClaims, permission string) bool {
	// Administradores con rol "super_admin" tienen todos los permisos
	if claims.Role == "super_admin" {
		return true
	}

	// Verificar permiso específico
	for _, perm := range claims.Permissions {
		if perm == permission || perm == "*" {
			return true
		}
	}

	return false
}

// RevokeToken agrega token a lista de tokens revocados
func (j *JWTManager) RevokeToken(tokenString string) error {
	claims, err := j.ValidateAdminToken(tokenString)
	if err != nil {
		return err
	}

	// Almacenar JTI en Redis con tiempo de expiración igual al del token
	if j.redisClient != nil {
		ttl := time.Until(claims.ExpiresAt.Time)
		if ttl > 0 {
			ctx := context.Background()
			key := fmt.Sprintf("revoked:%s", claims.ID)
			if err := j.redisClient.Set(ctx, key, "1", ttl).Err(); err != nil {
				return fmt.Errorf("failed to revoke token in Redis: %w", err)
			}
		}
	}

	return nil
}

// IsTokenRevoked verifica si token está revocado
func (j *JWTManager) IsTokenRevoked(jti string) bool {
	if j.redisClient == nil {
		return false // Sin Redis, no hay revocación
	}

	ctx := context.Background()
	key := fmt.Sprintf("revoked:%s", jti)
	exists, err := j.redisClient.Exists(ctx, key).Result()
	if err != nil {
		return false // En caso de error, permitir (fail-open)
	}

	return exists > 0
}

// generateJTI genera ID único para el token
func generateJTI() string {
	return fmt.Sprintf("%d_%d", time.Now().Unix(), time.Now().Nanosecond())
}

// AdminPermissions define permisos disponibles para administradores
var AdminPermissions = struct {
	// Permisos de auditoría legal
	LegalAuditRead        string
	LegalAuditExport      string
	LegalAuditVerify      string
	LegalAuditArchive     string
	LegalAuditStats       string

	// Permisos de sistema
	SystemAdmin           string
	UserManagement        string
	CompanyManagement     string
	APIKeyManagement      string

	// Permisos especiales
	SuperAdmin            string
	SecurityAudit         string
}{
	LegalAuditRead:        "legal_audit:read",
	LegalAuditExport:      "legal_audit:export",
	LegalAuditVerify:      "legal_audit:verify",
	LegalAuditArchive:     "legal_audit:archive",
	LegalAuditStats:       "legal_audit:stats",

	SystemAdmin:           "system:admin",
	UserManagement:        "users:manage",
	CompanyManagement:     "companies:manage",
	APIKeyManagement:      "api_keys:manage",

	SuperAdmin:            "*",
	SecurityAudit:         "security:audit",
}

// GetStandardPermissions retorna permisos estándar por rol
func GetStandardPermissions(role string) []string {
	switch role {
	case "super_admin":
		return []string{AdminPermissions.SuperAdmin}

	case "legal_admin":
		return []string{
			AdminPermissions.LegalAuditRead,
			AdminPermissions.LegalAuditExport,
			AdminPermissions.LegalAuditVerify,
			AdminPermissions.LegalAuditStats,
		}

	case "security_admin":
		return []string{
			AdminPermissions.SecurityAudit,
			AdminPermissions.LegalAuditRead,
			AdminPermissions.LegalAuditVerify,
		}

	case "system_admin":
		return []string{
			AdminPermissions.SystemAdmin,
			AdminPermissions.UserManagement,
			AdminPermissions.CompanyManagement,
			AdminPermissions.APIKeyManagement,
		}

	case "auditor":
		return []string{
			AdminPermissions.LegalAuditRead,
			AdminPermissions.LegalAuditStats,
		}

	default:
		return []string{} // Sin permisos por defecto
	}
}

// TokenInfo proporciona información sobre un token
type TokenInfo struct {
	UserID      int64     `json:"user_id"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	JTI         string    `json:"jti"`
	Valid       bool      `json:"valid"`
}

// GetTokenInfo extrae información de un token sin validarlo completamente
func (j *JWTManager) GetTokenInfo(tokenString string) (*TokenInfo, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&AdminClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return j.secretKey, nil
		},
	)

	if err != nil {
		return &TokenInfo{Valid: false}, nil
	}

	claims, ok := token.Claims.(*AdminClaims)
	if !ok {
		return &TokenInfo{Valid: false}, nil
	}

	info := &TokenInfo{
		UserID:      claims.UserID,
		Email:       claims.Email,
		Role:        claims.Role,
		Permissions: claims.Permissions,
		JTI:         claims.ID,
		Valid:       token.Valid,
	}

	if claims.IssuedAt != nil {
		info.IssuedAt = claims.IssuedAt.Time
	}

	if claims.ExpiresAt != nil {
		info.ExpiresAt = claims.ExpiresAt.Time
	}

	return info, nil
}