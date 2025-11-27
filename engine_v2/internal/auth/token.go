package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("expired token")
	ErrRevokedToken     = errors.New("revoked token")
	ErrInvalidSignature = errors.New("invalid signature")
)

// TokenManager gestiona tokens JWT con seguridad mejorada
type TokenManager struct {
	logger             *logger.Logger
	redis              *redis.Client
	accessTokenSecret  string
	refreshTokenSecret string
	accessTokenTTL     time.Duration
	refreshTokenTTL    time.Duration
}

// TokenPair contiene access token y refresh token
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Claims estructura de claims JWT
type Claims struct {
	UserID   string   `json:"user_id"`
	Email    string   `json:"email"`
	Plan     string   `json:"plan"`
	Roles    []string `json:"roles"`
	TokenID  string   `json:"jti"` // JWT ID para revocación
	IssuedAt int64    `json:"iat"`
	jwt.RegisteredClaims
}

// NewTokenManager crea un nuevo gestor de tokens
func NewTokenManager(
	log *logger.Logger,
	redisClient *redis.Client,
	accessSecret, refreshSecret string,
	accessTTL, refreshTTL time.Duration,
) *TokenManager {
	// TTL por defecto: access 15min, refresh 7 días
	if accessTTL == 0 {
		accessTTL = 15 * time.Minute
	}
	if refreshTTL == 0 {
		refreshTTL = 7 * 24 * time.Hour
	}

	return &TokenManager{
		logger:             log,
		redis:              redisClient,
		accessTokenSecret:  accessSecret,
		refreshTokenSecret: refreshSecret,
		accessTokenTTL:     accessTTL,
		refreshTokenTTL:    refreshTTL,
	}
}

// GenerateTokenPair genera access token y refresh token
func (tm *TokenManager) GenerateTokenPair(ctx context.Context, userID, email, plan string, roles []string) (*TokenPair, error) {
	now := time.Now()

	// Generar access token
	accessTokenID := tm.generateTokenID()
	accessToken, err := tm.generateAccessToken(userID, email, plan, roles, accessTokenID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generar refresh token
	refreshTokenID := tm.generateTokenID()
	refreshToken, err := tm.generateRefreshToken(userID, refreshTokenID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Almacenar en Redis para tracking y revocación
	if err := tm.storeTokenPair(ctx, userID, accessTokenID, refreshTokenID); err != nil {
		tm.logger.Error("Failed to store token pair in Redis", "error", err)
		// No fallar, pero loguear
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(tm.accessTokenTTL.Seconds()),
		ExpiresAt:    now.Add(tm.accessTokenTTL),
	}, nil
}

// generateAccessToken genera un access token JWT
func (tm *TokenManager) generateAccessToken(userID, email, plan string, roles []string, tokenID string, now time.Time) (string, error) {
	claims := &Claims{
		UserID:  userID,
		Email:   email,
		Plan:    plan,
		Roles:   roles,
		TokenID: tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(tm.accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "tucentropdf",
			ID:        tokenID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(tm.accessTokenSecret))
}

// generateRefreshToken genera un refresh token JWT
func (tm *TokenManager) generateRefreshToken(userID, tokenID string, now time.Time) (string, error) {
	claims := &jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(now.Add(tm.refreshTokenTTL)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		Issuer:    "tucentropdf",
		ID:        tokenID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(tm.refreshTokenSecret))
}

// ValidateAccessToken valida un access token
func (tm *TokenManager) ValidateAccessToken(ctx context.Context, tokenString string) (*Claims, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validar algoritmo
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tm.accessTokenSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, ErrInvalidSignature
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Verificar si está revocado
	isRevoked, err := tm.isTokenRevoked(ctx, claims.TokenID)
	if err != nil {
		tm.logger.Error("Failed to check token revocation", "error", err)
		// No fallar por error de Redis, pero loguear
	}
	if isRevoked {
		return nil, ErrRevokedToken
	}

	return claims, nil
}

// ValidateRefreshToken valida un refresh token
func (tm *TokenManager) ValidateRefreshToken(ctx context.Context, tokenString string) (string, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tm.refreshTokenSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpiredToken
		}
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return "", ErrInvalidSignature
		}
		return "", ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidToken
	}

	// Verificar si está revocado
	isRevoked, err := tm.isTokenRevoked(ctx, claims.ID)
	if err != nil {
		tm.logger.Error("Failed to check refresh token revocation", "error", err)
	}
	if isRevoked {
		return "", ErrRevokedToken
	}

	return claims.Subject, nil
}

// RefreshTokens rota tokens (revoca refresh anterior y genera nuevos)
func (tm *TokenManager) RefreshTokens(ctx context.Context, refreshTokenString string) (*TokenPair, error) {
	// Validar refresh token
	userID, err := tm.ValidateRefreshToken(ctx, refreshTokenString)
	if err != nil {
		return nil, err
	}

	// TODO: Obtener info del usuario desde DB (email, plan, roles)
	// Por ahora usamos placeholder
	email := ""    // Fetch from DB
	plan := "free" // Fetch from DB
	roles := []string{"user"}

	// Parse token anterior para obtener token ID
	token, _ := jwt.ParseWithClaims(refreshTokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(tm.refreshTokenSecret), nil
	})
	if token != nil {
		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if ok {
			// Revocar refresh token anterior (token rotation)
			if err := tm.RevokeToken(ctx, claims.ID); err != nil {
				tm.logger.Error("Failed to revoke old refresh token", "error", err)
			}
		}
	}

	// Generar nuevo par de tokens
	return tm.GenerateTokenPair(ctx, userID, email, plan, roles)
}

// RevokeToken revoca un token (blacklist)
func (tm *TokenManager) RevokeToken(ctx context.Context, tokenID string) error {
	key := fmt.Sprintf("token:revoked:%s", tokenID)

	// TTL = max(accessTTL, refreshTTL) para asegurar que expire
	ttl := tm.refreshTokenTTL
	if tm.accessTokenTTL > ttl {
		ttl = tm.accessTokenTTL
	}

	return tm.redis.Set(ctx, key, "1", ttl).Err()
}

// RevokeAllUserTokens revoca todos los tokens de un usuario
func (tm *TokenManager) RevokeAllUserTokens(ctx context.Context, userID string) error {
	// Obtener todos los token IDs del usuario
	pattern := fmt.Sprintf("token:user:%s:*", userID)
	keys, err := tm.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get user tokens: %w", err)
	}

	// Revocar cada token
	for _, key := range keys {
		tokenID, err := tm.redis.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		if err := tm.RevokeToken(ctx, tokenID); err != nil {
			tm.logger.Error("Failed to revoke token", "token_id", tokenID, "error", err)
		}
	}

	// Eliminar tracking keys
	if len(keys) > 0 {
		tm.redis.Del(ctx, keys...)
	}

	return nil
}

// isTokenRevoked verifica si un token está en blacklist
func (tm *TokenManager) isTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	key := fmt.Sprintf("token:revoked:%s", tokenID)
	exists, err := tm.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// storeTokenPair almacena el par de tokens para tracking
func (tm *TokenManager) storeTokenPair(ctx context.Context, userID, accessTokenID, refreshTokenID string) error {
	// Almacenar mapping user -> tokens
	accessKey := fmt.Sprintf("token:user:%s:access:%s", userID, accessTokenID)
	refreshKey := fmt.Sprintf("token:user:%s:refresh:%s", userID, refreshTokenID)

	pipe := tm.redis.Pipeline()
	pipe.Set(ctx, accessKey, accessTokenID, tm.accessTokenTTL)
	pipe.Set(ctx, refreshKey, refreshTokenID, tm.refreshTokenTTL)

	_, err := pipe.Exec(ctx)
	return err
}

// generateTokenID genera un ID único para el token
func (tm *TokenManager) generateTokenID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// GetUserActiveSessions retorna las sesiones activas de un usuario
func (tm *TokenManager) GetUserActiveSessions(ctx context.Context, userID string) (int, error) {
	pattern := fmt.Sprintf("token:user:%s:*", userID)
	keys, err := tm.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return 0, err
	}

	// Dividir por 2 (cada sesión tiene access + refresh)
	return len(keys) / 2, nil
}

// CleanupExpiredTokens limpia tokens expirados del tracking (cron job)
func (tm *TokenManager) CleanupExpiredTokens(ctx context.Context) error {
	// Redis ya expira automáticamente con TTL
	// Esta función es opcional para limpiar manualmente si es necesario

	pattern := "token:user:*"
	cursor := uint64(0)
	deleted := 0

	for {
		keys, nextCursor, err := tm.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			ttl, err := tm.redis.TTL(ctx, key).Result()
			if err != nil {
				continue
			}

			// Si TTL es negativo, la key no existe o no tiene TTL
			if ttl < 0 {
				tm.redis.Del(ctx, key)
				deleted++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if deleted > 0 {
		tm.logger.Info("Cleaned up expired tokens", "count", deleted)
	}

	return nil
}

// ValidateTokenWithIPBinding valida token y verifica IP binding
func (tm *TokenManager) ValidateTokenWithIPBinding(ctx context.Context, tokenString, currentIP string) (*Claims, error) {
	claims, err := tm.ValidateAccessToken(ctx, tokenString)
	if err != nil {
		return nil, err
	}

	// Verificar IP binding (opcional, puede ser muy restrictivo)
	storedIP, err := tm.getTokenIP(ctx, claims.TokenID)
	if err == nil && storedIP != "" && storedIP != currentIP {
		tm.logger.Warn("Token IP mismatch",
			"user_id", claims.UserID,
			"stored_ip", storedIP,
			"current_ip", currentIP,
		)
		// Opcional: revocar token
		// return nil, errors.New("IP mismatch")
	}

	return claims, nil
}

// storeTokenIP almacena la IP asociada a un token
func (tm *TokenManager) storeTokenIP(ctx context.Context, tokenID, ip string) error {
	key := fmt.Sprintf("token:ip:%s", tokenID)
	return tm.redis.Set(ctx, key, ip, tm.accessTokenTTL).Err()
}

// getTokenIP obtiene la IP asociada a un token
func (tm *TokenManager) getTokenIP(ctx context.Context, tokenID string) (string, error) {
	key := fmt.Sprintf("token:ip:%s", tokenID)
	return tm.redis.Get(ctx, key).Result()
}
