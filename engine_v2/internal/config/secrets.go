package config

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

var (
	ErrSecretNotFound = errors.New("secret not found")
	ErrInvalidKey     = errors.New("invalid encryption key")
)

// SecretsManager gestiona secretos encriptados
type SecretsManager struct {
	logger       *logger.Logger
	redis        *redis.Client
	encryptionKey []byte
	cache        sync.Map
	cacheTTL     time.Duration
	vaultEnabled bool
	vaultConfig  *VaultConfig
}

// VaultConfig configuración para HashiCorp Vault
type VaultConfig struct {
	Address   string
	Token     string
	Namespace string
	MountPath string
	Enabled   bool
}

// Secret representa un secreto almacenado
type Secret struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Encrypted bool      `json:"encrypted"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewSecretsManager crea un nuevo gestor de secretos
func NewSecretsManager(
	log *logger.Logger,
	redisClient *redis.Client,
	encryptionKey string,
	vaultConfig *VaultConfig,
) (*SecretsManager, error) {
	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(encryptionKey))
	}

	sm := &SecretsManager{
		logger:        log,
		redis:         redisClient,
		encryptionKey: []byte(encryptionKey),
		cacheTTL:      5 * time.Minute,
		vaultEnabled:  vaultConfig != nil && vaultConfig.Enabled,
		vaultConfig:   vaultConfig,
	}

	return sm, nil
}

// Get obtiene un secreto (cache -> Redis -> Vault -> env)
func (sm *SecretsManager) Get(ctx context.Context, key string) (string, error) {
	// 1. Buscar en cache local
	if cached, ok := sm.cache.Load(key); ok {
		secret := cached.(*Secret)
		// Verificar si expiró
		if !secret.ExpiresAt.IsZero() && time.Now().After(secret.ExpiresAt) {
			sm.cache.Delete(key)
		} else {
			return secret.Value, nil
		}
	}

	// 2. Buscar en Redis
	secret, err := sm.getFromRedis(ctx, key)
	if err == nil {
		sm.cache.Store(key, secret)
		return secret.Value, nil
	}

	// 3. Buscar en Vault (si está habilitado)
	if sm.vaultEnabled {
		value, err := sm.getFromVault(ctx, key)
		if err == nil {
			// Cachear en Redis y local
			secret := &Secret{
				Key:       key,
				Value:     value,
				Encrypted: false,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			sm.storeInRedis(ctx, secret)
			sm.cache.Store(key, secret)
			return value, nil
		}
	}

	// 4. Buscar en variables de entorno (fallback)
	envKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if value := os.Getenv(envKey); value != "" {
		return value, nil
	}

	return "", ErrSecretNotFound
}

// Set almacena un secreto encriptado
func (sm *SecretsManager) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	// Encriptar valor
	encryptedValue, err := sm.encrypt(value)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	secret := &Secret{
		Key:       key,
		Value:     value, // Guardamos desencriptado en memoria
		Encrypted: true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if ttl > 0 {
		secret.ExpiresAt = time.Now().Add(ttl)
	}

	// Almacenar en Redis (encriptado)
	redisKey := fmt.Sprintf("secret:%s", key)
	secretData, _ := json.Marshal(&Secret{
		Key:       key,
		Value:     encryptedValue, // Valor encriptado
		Encrypted: true,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
		ExpiresAt: secret.ExpiresAt,
	})

	if ttl > 0 {
		err = sm.redis.Set(ctx, redisKey, secretData, ttl).Err()
	} else {
		err = sm.redis.Set(ctx, redisKey, secretData, 0).Err()
	}

	if err != nil {
		return fmt.Errorf("failed to store secret in Redis: %w", err)
	}

	// Cachear en memoria
	sm.cache.Store(key, secret)

	// Opcional: almacenar en Vault
	if sm.vaultEnabled {
		if err := sm.storeInVault(ctx, key, value); err != nil {
			sm.logger.Warn("Failed to store secret in Vault", "key", key, "error", err)
			// No fallar si Vault falla
		}
	}

	return nil
}

// Delete elimina un secreto
func (sm *SecretsManager) Delete(ctx context.Context, key string) error {
	// Eliminar de cache
	sm.cache.Delete(key)

	// Eliminar de Redis
	redisKey := fmt.Sprintf("secret:%s", key)
	if err := sm.redis.Del(ctx, redisKey).Err(); err != nil {
		return fmt.Errorf("failed to delete secret from Redis: %w", err)
	}

	// Opcional: eliminar de Vault
	if sm.vaultEnabled {
		if err := sm.deleteFromVault(ctx, key); err != nil {
			sm.logger.Warn("Failed to delete secret from Vault", "key", key, "error", err)
		}
	}

	return nil
}

// Rotate rota un secreto (genera nuevo valor y revoca anterior)
func (sm *SecretsManager) Rotate(ctx context.Context, key string, newValue string) error {
	// Obtener secreto anterior
	oldValue, err := sm.Get(ctx, key)
	if err != nil && !errors.Is(err, ErrSecretNotFound) {
		return err
	}

	// Almacenar nuevo valor
	if err := sm.Set(ctx, key, newValue, 0); err != nil {
		return err
	}

	// Almacenar valor anterior con TTL (para rollback)
	rotationKey := fmt.Sprintf("%s.old.%d", key, time.Now().Unix())
	if oldValue != "" {
		if err := sm.Set(ctx, rotationKey, oldValue, 24*time.Hour); err != nil {
			sm.logger.Warn("Failed to store old secret", "key", rotationKey, "error", err)
		}
	}

	sm.logger.Info("Secret rotated", "key", key)

	return nil
}

// getFromRedis obtiene secreto de Redis
func (sm *SecretsManager) getFromRedis(ctx context.Context, key string) (*Secret, error) {
	redisKey := fmt.Sprintf("secret:%s", key)
	data, err := sm.redis.Get(ctx, redisKey).Result()
	if err != nil {
		return nil, err
	}

	var secret Secret
	if err := json.Unmarshal([]byte(data), &secret); err != nil {
		return nil, err
	}

	// Desencriptar si está encriptado
	if secret.Encrypted {
		decrypted, err := sm.decrypt(secret.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret: %w", err)
		}
		secret.Value = decrypted
	}

	return &secret, nil
}

// storeInRedis almacena secreto en Redis
func (sm *SecretsManager) storeInRedis(ctx context.Context, secret *Secret) error {
	redisKey := fmt.Sprintf("secret:%s", secret.Key)
	data, _ := json.Marshal(secret)

	ttl := time.Duration(0)
	if !secret.ExpiresAt.IsZero() {
		ttl = time.Until(secret.ExpiresAt)
	}

	return sm.redis.Set(ctx, redisKey, data, ttl).Err()
}

// encrypt encripta un valor usando AES-256-GCM
func (sm *SecretsManager) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(sm.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt desencripta un valor usando AES-256-GCM
func (sm *SecretsManager) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(sm.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonceBytes, cipherBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonceBytes, cipherBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// getFromVault obtiene secreto de HashiCorp Vault
func (sm *SecretsManager) getFromVault(ctx context.Context, key string) (string, error) {
	if !sm.vaultEnabled {
		return "", errors.New("Vault not enabled")
	}

	// TODO: Implementar integración real con Vault
	// Esto es un stub
	sm.logger.Debug("Fetching secret from Vault", "key", key)

	return "", ErrSecretNotFound
}

// storeInVault almacena secreto en HashiCorp Vault
func (sm *SecretsManager) storeInVault(ctx context.Context, key, value string) error {
	if !sm.vaultEnabled {
		return errors.New("Vault not enabled")
	}

	// TODO: Implementar integración real con Vault
	sm.logger.Debug("Storing secret in Vault", "key", key)

	return nil
}

// deleteFromVault elimina secreto de HashiCorp Vault
func (sm *SecretsManager) deleteFromVault(ctx context.Context, key string) error {
	if !sm.vaultEnabled {
		return errors.New("Vault not enabled")
	}

	// TODO: Implementar integración real con Vault
	sm.logger.Debug("Deleting secret from Vault", "key", key)

	return nil
}

// RefreshCache recarga secretos en cache
func (sm *SecretsManager) RefreshCache(ctx context.Context) error {
	// Obtener todas las keys de secretos en Redis
	pattern := "secret:*"
	keys, err := sm.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	refreshed := 0
	for _, redisKey := range keys {
		key := strings.TrimPrefix(redisKey, "secret:")
		
		// Cargar en cache
		secret, err := sm.getFromRedis(ctx, key)
		if err != nil {
			sm.logger.Warn("Failed to refresh secret", "key", key, "error", err)
			continue
		}

		sm.cache.Store(key, secret)
		refreshed++
	}

	sm.logger.Info("Secrets cache refreshed", "count", refreshed)

	return nil
}

// GetAllSecretKeys retorna todas las keys de secretos
func (sm *SecretsManager) GetAllSecretKeys(ctx context.Context) ([]string, error) {
	pattern := "secret:*"
	keys, err := sm.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	var secretKeys []string
	for _, key := range keys {
		secretKeys = append(secretKeys, strings.TrimPrefix(key, "secret:"))
	}

	return secretKeys, nil
}

// ValidateSecretsHealth verifica que secretos críticos existan
func (sm *SecretsManager) ValidateSecretsHealth(ctx context.Context, requiredSecrets []string) error {
	missing := []string{}

	for _, key := range requiredSecrets {
		_, err := sm.Get(ctx, key)
		if err != nil {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required secrets: %v", missing)
	}

	return nil
}

// EncryptEnvFile encripta un archivo .env
func EncryptEnvFile(inputPath, outputPath, encryptionKey string) error {
	// Leer archivo
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Crear cipher
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	// Encriptar
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	// Escribir archivo encriptado
	return os.WriteFile(outputPath, []byte(encoded), 0600)
}

// DecryptEnvFile desencripta un archivo .env
func DecryptEnvFile(inputPath, outputPath, encryptionKey string) error {
	// Leer archivo encriptado
	encoded, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Decodificar base64
	ciphertext, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return err
	}

	// Crear cipher
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return errors.New("ciphertext too short")
	}

	nonceBytes, cipherBytes := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonceBytes, cipherBytes, nil)
	if err != nil {
		return err
	}

	// Escribir archivo desencriptado
	return os.WriteFile(outputPath, plaintext, 0600)
}

// GenerateEncryptionKey genera una nueva clave de encriptación
func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
