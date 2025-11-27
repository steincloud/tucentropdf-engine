package legal_audit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/pbkdf2"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// LegalEncryptor maneja cifrado AES256 para campos sensibles de auditoría
type LegalEncryptor struct {
	masterKey []byte
	logger    *logger.Logger
}

// NewLegalEncryptor crea nueva instancia del encriptador legal
func NewLegalEncryptor(masterKey string, log *logger.Logger) *LegalEncryptor {
	// Derivar clave específica para auditoría legal usando salt específico
	salt := []byte("tucentropdf_legal_audit_salt_2025_v2")
	derivedKey := pbkdf2.Key([]byte(masterKey), salt, 100000, 32, sha256.New)

	return &LegalEncryptor{
		masterKey: derivedKey,
		logger:    log,
	}
}

// EncryptSensitiveField cifra un campo sensible para almacenamiento legal
func (e *LegalEncryptor) EncryptSensitiveField(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	e.logger.Debug("Encrypting sensitive legal field")

	// Cifrar datos
	ciphertext, err := e.encrypt([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("legal encryption failed: %w", err)
	}

	// Codificar en base64 para almacenamiento
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSensitiveField descifra un campo sensible
func (e *LegalEncryptor) DecryptSensitiveField(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	e.logger.Debug("Decrypting sensitive legal field")

	// Decodificar de base64
	encrypted, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	// Descifrar datos
	plaintext, err := e.decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("legal decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// encrypt cifra datos usando AES256-GCM
func (e *LegalEncryptor) encrypt(plaintext []byte) ([]byte, error) {
	// Crear cipher AES
	block, err := aes.NewCipher(e.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Crear GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generar nonce aleatorio
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Cifrar con autenticación
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// decrypt descifra datos usando AES256-GCM
func (e *LegalEncryptor) decrypt(ciphertext []byte) ([]byte, error) {
	// Crear cipher AES
	block, err := aes.NewCipher(e.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Crear GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Verificar tamaño mínimo
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extraer nonce y datos cifrados
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Descifrar con verificación de autenticidad
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptMetadata cifra metadatos completos en JSONB
func (e *LegalEncryptor) EncryptMetadata(metadata *JSONBMetadata) (*JSONBMetadata, error) {
	if metadata == nil {
		return nil, nil
	}

	encryptedMetadata := &JSONBMetadata{}
	*encryptedMetadata = *metadata

	// Cifrar campos sensibles específicos
	sensitiveFields := map[string]*string{
		"original_name": &metadata.OriginalName,
		"domain":        &metadata.Domain,
		"geo_location":  &metadata.GeoLocation,
	}

	for fieldName, fieldPtr := range sensitiveFields {
		if fieldPtr != nil && *fieldPtr != "" {
			encrypted, err := e.EncryptSensitiveField(*fieldPtr)
			if err != nil {
				e.logger.Error("Failed to encrypt metadata field", "field", fieldName, "error", err)
				continue
			}

			// Actualizar el campo en la metadata cifrada
			switch fieldName {
			case "original_name":
				encryptedMetadata.OriginalName = encrypted
			case "domain":
				encryptedMetadata.Domain = encrypted
			case "geo_location":
				encryptedMetadata.GeoLocation = encrypted
			}
		}
	}

	// Marcar como cifrado en extra
	if encryptedMetadata.Extra == nil {
		encryptedMetadata.Extra = make(map[string]interface{})
	}
	encryptedMetadata.Extra["encrypted_fields"] = []string{"original_name", "domain", "geo_location"}
	encryptedMetadata.Extra["encryption_timestamp"] = "now()"

	return encryptedMetadata, nil
}

// DecryptMetadata descifra metadatos completos
func (e *LegalEncryptor) DecryptMetadata(metadata *JSONBMetadata) (*JSONBMetadata, error) {
	if metadata == nil {
		return nil, nil
	}

	// Verificar si tiene campos cifrados
	encryptedFields, ok := metadata.Extra["encrypted_fields"].([]string)
	if !ok || len(encryptedFields) == 0 {
		// No está cifrado, retornar tal como está
		return metadata, nil
	}

	decryptedMetadata := &JSONBMetadata{}
	*decryptedMetadata = *metadata

	// Descifrar campos sensibles
	for _, fieldName := range encryptedFields {
		switch fieldName {
		case "original_name":
			if metadata.OriginalName != "" {
				decrypted, err := e.DecryptSensitiveField(metadata.OriginalName)
				if err != nil {
					e.logger.Error("Failed to decrypt metadata field", "field", fieldName, "error", err)
					continue
				}
				decryptedMetadata.OriginalName = decrypted
			}
		case "domain":
			if metadata.Domain != "" {
				decrypted, err := e.DecryptSensitiveField(metadata.Domain)
				if err != nil {
					e.logger.Error("Failed to decrypt metadata field", "field", fieldName, "error", err)
					continue
				}
				decryptedMetadata.Domain = decrypted
			}
		case "geo_location":
			if metadata.GeoLocation != "" {
				decrypted, err := e.DecryptSensitiveField(metadata.GeoLocation)
				if err != nil {
					e.logger.Error("Failed to decrypt metadata field", "field", fieldName, "error", err)
					continue
				}
				decryptedMetadata.GeoLocation = decrypted
			}
		}
	}

	// Limpiar metadata de información de cifrado para el resultado final
	if decryptedMetadata.Extra != nil {
		delete(decryptedMetadata.Extra, "encrypted_fields")
		delete(decryptedMetadata.Extra, "encryption_timestamp")
	}

	return decryptedMetadata, nil
}

// GenerateFieldHash genera hash SHA256 para verificación de integridad de campos
func (e *LegalEncryptor) GenerateFieldHash(data string) string {
	if data == "" {
		return ""
	}

	hasher := sha256.New()
	hasher.Write([]byte(data))
	return hex.EncodeToString(hasher.Sum(nil))
}

// ValidateEncryption valida que el sistema de cifrado funcione correctamente
func (e *LegalEncryptor) ValidateEncryption() error {
	// Probar cifrado/descifrado con datos de prueba
	testData := "TuCentroPDF Legal Audit Encryption Test Data"

	// Cifrar
	encrypted, err := e.EncryptSensitiveField(testData)
	if err != nil {
		return fmt.Errorf("encryption test failed: %w", err)
	}

	// Descifrar
	decrypted, err := e.DecryptSensitiveField(encrypted)
	if err != nil {
		return fmt.Errorf("decryption test failed: %w", err)
	}

	// Verificar que los datos coincidan
	if testData != decrypted {
		return fmt.Errorf("encryption/decryption test data mismatch")
	}

	e.logger.Debug("Legal encryption validation successful")
	return nil
}

// EncryptExportFile cifra archivo completo de exportación legal
func (e *LegalEncryptor) EncryptExportFile(filePath string) (string, error) {
	// Leer archivo original
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Cifrar contenido
	ciphertext, err := e.encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt file: %w", err)
	}

	// Escribir archivo cifrado
	encryptedPath := filePath + ".enc"
	if err := os.WriteFile(encryptedPath, ciphertext, 0600); err != nil {
		return "", fmt.Errorf("failed to write encrypted file: %w", err)
	}

	return encryptedPath, nil
}

// DecryptExportFile descifra archivo de exportación legal
func (e *LegalEncryptor) DecryptExportFile(encryptedPath, outputPath string) error {
	// Leer archivo cifrado
	ciphertext, err := os.ReadFile(encryptedPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %w", err)
	}

	// Descifrar contenido
	plaintext, err := e.decrypt(ciphertext)
	if err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	}

	// Escribir archivo descifrado
	if err := os.WriteFile(outputPath, plaintext, 0640); err != nil {
		return fmt.Errorf("failed to write decrypted file: %w", err)
	}

	return nil
}

// IsFieldEncrypted verifica si un campo parece estar cifrado
func (e *LegalEncryptor) IsFieldEncrypted(data string) bool {
	// Un campo cifrado en base64 debería tener cierta longitud mínima
	// y solo caracteres base64 válidos
	if len(data) < 32 {
		return false
	}

	// Intentar decodificar base64
	_, err := base64.StdEncoding.DecodeString(data)
	return err == nil
}

// GetEncryptionInfo retorna información sobre el estado de cifrado
func (e *LegalEncryptor) GetEncryptionInfo() map[string]interface{} {
	return map[string]interface{}{
		"algorithm":    "AES256-GCM",
		"key_derivation": "PBKDF2-SHA256",
		"iterations":   100000,
		"key_length":   len(e.masterKey),
		"nonce_size":   12, // GCM estándar
		"tag_size":     16, // GCM estándar
		"status":       "active",
	}
}