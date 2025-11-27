package middleware

import (
    "bytes"
    "errors"
    "mime/multipart"
    "path/filepath"
    "regexp"
    "strings"

    "github.com/gofiber/fiber/v2"
    "github.com/tucentropdf/engine-v2/pkg/logger"
)

// SecurityMiddleware compat shim para tests y código legacy.
// Provee `ValidateFileType` y `SanitizeFilename` usados en tests.
type SecurityMiddleware struct {
    log *logger.Logger
}

// NewSecurityMiddleware crea la instancia shim
func NewSecurityMiddleware(log *logger.Logger) *SecurityMiddleware {
    return &SecurityMiddleware{log: log}
}

// ValidateFileType valida tipos de archivo sencillos basados en headers/contenido
func (s *SecurityMiddleware) ValidateFileType() fiber.Handler {
    return func(c *fiber.Ctx) error {
        // Si no es multipart, permitir (otros middlewares se encargan)
        contentType := c.Get("Content-Type")
        if !strings.Contains(contentType, "multipart/form-data") {
            // Si el content-type declara PDF pero el body no parece PDF, rechazar
            if strings.Contains(strings.ToLower(contentType), "application/pdf") {
                // body header check
                b := c.Body()
                if len(b) < 4 || string(b[:4]) != "%PDF" {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid PDF content"})
                }
            }
            return c.Next()
        }

        form, err := c.MultipartForm()
        if err != nil {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid multipart form"})
        }

        processed := false

        for _, files := range form.File {
            for _, fh := range files {
                processed = true
                fname := strings.ToLower(fh.Filename)
                if strings.HasSuffix(fname, ".exe") || strings.HasSuffix(fname, ".dll") {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "executable files not allowed"})
                }
                if strings.HasSuffix(fname, ".js") || strings.HasSuffix(fname, ".sh") || strings.HasSuffix(fname, ".py") {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "script files not allowed"})
                }
                // Validación por tipo declarado y encabezado binario
                claimed := fh.Header.Get("Content-Type")
                if err := checkFileHeaderStrict(fh, claimed); err != nil {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
                }
            }
        }

        // Si Fiber no pobló form.File, parseamos manualmente el multipart
        if !processed {
            ct := c.Get("Content-Type")
            partsReader := multipart.NewReader(bytes.NewReader(c.Body()), parseBoundary(ct))
            for {
                part, perr := partsReader.NextPart()
                if perr != nil {
                    break
                }
                processed = true
                fname := strings.ToLower(part.FileName())
                if fname == "" {
                    continue
                }
                if strings.HasSuffix(fname, ".exe") || strings.HasSuffix(fname, ".dll") {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "executable files not allowed"})
                }
                if strings.HasSuffix(fname, ".js") || strings.HasSuffix(fname, ".sh") || strings.HasSuffix(fname, ".py") {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "script files not allowed"})
                }
                // Leer los primeros bytes
                buf := make([]byte, 8)
                n, _ := part.Read(buf)
                claimed := part.Header.Get("Content-Type")
                if err := checkBufferStrict(buf[:n], claimed); err != nil {
                    return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
                }
            }
        }

        if !processed {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no file parts found"})
        }

        return c.Next()
    }
}

func checkFileHeaderStrict(fh *multipart.FileHeader, claimedType string) error {
    f, err := fh.Open()
    if err != nil {
        return errors.New("cannot open uploaded file")
    }
    defer f.Close()

    buf := make([]byte, 8)
    n, _ := f.Read(buf)
    if n >= 4 && string(buf[:4]) == "%PDF" {
        // if claims pdf, ok; else allow
        return nil
    }
    // JPEG
    if n >= 3 && buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF {
        return nil
    }
    // PE (MZ)
    if n >= 2 && buf[0] == 'M' && buf[1] == 'Z' {
        return errors.New("executable files not allowed")
    }

    // Content-type mismatch checks
    ct := strings.ToLower(claimedType)
    if strings.Contains(ct, "application/pdf") {
        return errors.New("invalid PDF content")
    }
    if strings.Contains(ct, "image/") {
        // for images, require JPEG/PNG headers minimally
        if !(n >= 3 && buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF) { // jpeg
            // naive PNG check
            if !(n >= 8 && buf[0] == 0x89 && buf[1] == 'P' && buf[2] == 'N' && buf[3] == 'G') {
                return errors.New("invalid image content")
            }
        }
        return nil
    }
    // Fallback: if unknown type, allow for now
    return nil
}

func checkBufferStrict(buf []byte, claimedType string) error {
    n := len(buf)
    if n >= 4 && string(buf[:4]) == "%PDF" {
        return nil
    }
    if n >= 3 && buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF {
        return nil
    }
    if n >= 2 && buf[0] == 'M' && buf[1] == 'Z' {
        return errors.New("executable files not allowed")
    }
    ct := strings.ToLower(claimedType)
    if strings.Contains(ct, "application/pdf") {
        return errors.New("invalid PDF content")
    }
    if strings.Contains(ct, "image/") {
        if !(n >= 3 && buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF) {
            if !(n >= 8 && buf[0] == 0x89 && buf[1] == 'P' && buf[2] == 'N' && buf[3] == 'G') {
                return errors.New("invalid image content")
            }
        }
    }
    return nil
}

func parseBoundary(contentType string) string {
    // Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryabc123
    parts := strings.Split(contentType, ";")
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if strings.HasPrefix(strings.ToLower(p), "boundary=") {
            return strings.TrimPrefix(p, "boundary=")
        }
    }
    return ""
}

// SanitizeFilename limpia un nombre de archivo (eliminar rutas, caracteres peligrosos)
func (s *SecurityMiddleware) SanitizeFilename(name string) string {
    // Eliminar path
    base := filepath.Base(name)
    // Reemplazar espacios por guion bajo
    base = strings.ReplaceAll(base, " ", "_")
    // Eliminar caracteres no alfanuméricos salvo puntos y guiones bajos
    re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
    base = re.ReplaceAllString(base, "")
    // Normalizar unicode simplificada: eliminar tildes comunes (primitiva)
    base = strings.ReplaceAll(base, "ñ", "n")
    base = strings.ReplaceAll(base, "Ñ", "N")
    base = strings.ReplaceAll(base, "á", "a")
    base = strings.ReplaceAll(base, "é", "e")
    base = strings.ReplaceAll(base, "í", "i")
    base = strings.ReplaceAll(base, "ó", "o")
    base = strings.ReplaceAll(base, "ú", "u")
    return base
}

// validateFileSize funcion helper usada en tests (bytes, maxSizeMB)
func validateFileSize(fileSize int64, maxSizeMB int) error {
    if fileSize <= 0 {
        return errors.New("file size must be greater than zero")
    }
    maxBytes := int64(maxSizeMB) * 1024 * 1024
    if fileSize >= maxBytes {
        return errors.New("file exceeds max allowed size")
    }
    return nil
}
