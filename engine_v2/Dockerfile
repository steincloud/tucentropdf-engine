# Build stage
FROM golang:1.24-alpine AS builder

# Instalar dependencias de build
RUN apk add --no-cache git ca-certificates tzdata upx

# Establecer directorio de trabajo
WORKDIR /app

# Copiar go mod files primero para mejor caching
COPY go.mod go.sum ./

# Descargar dependencias
RUN go mod download && go mod verify

# Copiar código fuente
COPY . .

# Build del binario optimizado
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o main cmd/server/main.go

# Comprimir binario
RUN upx --ultra-brute -qq main && upx -t main

# Production stage
FROM alpine:3.19

# Metadatos
LABEL maintainer="TuCentroPDF Team"
LABEL version="2.0.0"
LABEL description="TuCentroPDF Engine V2 - PDF, Office & OCR Processing"

# Instalar dependencias de runtime
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    bash \
    # Tesseract OCR
    tesseract-ocr \
    tesseract-ocr-data-eng \
    tesseract-ocr-data-spa \
    tesseract-ocr-data-por \
    tesseract-ocr-data-fra \
    # LibreOffice
    libreoffice \
    # Fonts para Office
    ttf-dejavu \
    ttf-liberation \
    # Utilidades
    file \
    && rm -rf /var/cache/apk/* \
    && rm -rf /tmp/*

# Crear usuario no-root
RUN addgroup -g 1001 appgroup && \
    adduser -D -u 1001 -G appgroup appuser

# Crear directorio de trabajo y temp
WORKDIR /app
RUN mkdir -p /tmp/tucentropdf-v2 /app/logs && \
    chown -R appuser:appgroup /app /tmp/tucentropdf-v2

# Copiar binario desde build stage
COPY --from=builder --chown=appuser:appgroup /app/main ./

# Copiar archivos de configuración
COPY --from=builder --chown=appuser:appgroup /app/.env.example ./

# Cambiar ownership
RUN chown -R appuser:appuser /app

# Cambiar a usuario no-root
USER appuser

# Exponer puerto
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Variables de entorno por defecto
ENV PORT=8080
ENV ENVIRONMENT=production
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Comando por defecto
CMD ["./main"]