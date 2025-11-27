package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/pbkdf2"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Encryptor maneja el cifrado AES256-GCM para backups
type Encryptor struct {
	masterKey []byte
	logger    *logger.Logger
}

// NewEncryptor crea nueva instancia del encriptador
func NewEncryptor(masterKey string, log *logger.Logger) *Encryptor {
	// Derivar clave de 32 bytes usando PBKDF2
	salt := []byte("tucentropdf_backup_salt_2025") // Salt fijo para consistencia
	derivedKey := pbkdf2.Key([]byte(masterKey), salt, 100000, 32, sha256.New)

	return &Encryptor{
		masterKey: derivedKey,
		logger:    log,
	}
}

// EncryptFile cifra un archivo usando AES256-GCM
func (e *Encryptor) EncryptFile(inputPath, outputPath string) error {
	e.logger.Debug("Encrypting file", "input", inputPath, "output", outputPath)

	// Leer archivo original
	plaintext, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Cifrar datos
	ciphertext, err := e.encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	// Escribir archivo cifrado
	if err := os.WriteFile(outputPath, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted file: %w", err)
	}

	e.logger.Debug("File encrypted successfully", 
		"original_size", len(plaintext),
		"encrypted_size", len(ciphertext))

	return nil
}

// DecryptFile descifra un archivo usando AES256-GCM
func (e *Encryptor) DecryptFile(inputPath, outputPath string) error {
	e.logger.Debug("Decrypting file", "input", inputPath, "output", outputPath)

	// Leer archivo cifrado
	ciphertext, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %w", err)
	}

	// Descifrar datos
	plaintext, err := e.decrypt(ciphertext)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	// Escribir archivo descifrado
	if err := os.WriteFile(outputPath, plaintext, 0640); err != nil {
		return fmt.Errorf("failed to write decrypted file: %w", err)
	}

	e.logger.Debug("File decrypted successfully",
		"encrypted_size", len(ciphertext),
		"decrypted_size", len(plaintext))

	return nil
}

// encrypt cifra datos usando AES256-GCM
func (e *Encryptor) encrypt(plaintext []byte) ([]byte, error) {
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
func (e *Encryptor) decrypt(ciphertext []byte) ([]byte, error) {
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

// EncryptBytes cifra datos en memoria
func (e *Encryptor) EncryptBytes(data []byte) ([]byte, error) {
	return e.encrypt(data)
}

// DecryptBytes descifra datos en memoria
func (e *Encryptor) DecryptBytes(data []byte) ([]byte, error) {
	return e.decrypt(data)
}

// ValidateKey valida que la clave de cifrado funcione correctamente
func (e *Encryptor) ValidateKey() error {
	// Probar cifrado/descifrado con datos de prueba
	testData := []byte("TuCentroPDF Engine V2 Backup Encryption Test")

	// Cifrar
	encrypted, err := e.encrypt(testData)
	if err != nil {
		return fmt.Errorf("encryption test failed: %w", err)
	}

	// Descifrar
	decrypted, err := e.decrypt(encrypted)
	if err != nil {
		return fmt.Errorf("decryption test failed: %w", err)
	}

	// Verificar que los datos coincidan
	if string(testData) != string(decrypted) {
		return fmt.Errorf("encryption/decryption test data mismatch")
	}

	e.logger.Debug("Encryption key validation successful")
	return nil
}

// GetEncryptedExtension retorna la extensión para archivos cifrados
func (e *Encryptor) GetEncryptedExtension() string {
	return ".enc"
}

// IsFileEncrypted verifica si un archivo está cifrado (por extensión)
func (e *Encryptor) IsFileEncrypted(filename string) bool {
	return len(filename) > 4 && filename[len(filename)-4:] == ".enc"
}

// GetDecryptedFilename remueve la extensión .enc del nombre de archivo
func (e *Encryptor) GetDecryptedFilename(encryptedFilename string) string {
	if e.IsFileEncrypted(encryptedFilename) {
		return encryptedFilename[:len(encryptedFilename)-4]
	}
	return encryptedFilename
}