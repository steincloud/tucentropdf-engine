#!/bin/bash
# TuCentroPDF Engine V2 - Secrets Generator
# Generates secure random secrets for production deployment

set -e

echo "🔐 Generating secure secrets for TuCentroPDF Engine V2..."
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if .env.production exists
if [ -f .env.production ]; then
    echo -e "${YELLOW}⚠️  .env.production already exists${NC}"
    read -p "Do you want to regenerate secrets? This will overwrite existing values (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 0
    fi
    # Backup existing file
    cp .env.production .env.production.backup.$(date +%Y%m%d_%H%M%S)
    echo -e "${GREEN}✅ Backup created${NC}"
fi

# Generate ENGINE_SECRET (64 characters)
echo "Generating ENGINE_SECRET (64 chars)..."
ENGINE_SECRET=$(openssl rand -base64 48 | tr -d "=+/" | cut -c1-64)

# Generate JWT_SECRET (RSA 4096 bits)
echo "Generating JWT_SECRET (RSA 4096)..."
openssl genrsa -out /tmp/jwt_private.pem 4096 2>/dev/null
JWT_PRIVATE_KEY=$(cat /tmp/jwt_private.pem | base64 -w 0)
openssl rsa -in /tmp/jwt_private.pem -pubout -out /tmp/jwt_public.pem 2>/dev/null
JWT_PUBLIC_KEY=$(cat /tmp/jwt_public.pem | base64 -w 0)
rm -f /tmp/jwt_private.pem /tmp/jwt_public.pem

# Generate REDIS_PASSWORD (32 characters)
echo "Generating REDIS_PASSWORD (32 chars)..."
REDIS_PASSWORD=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-32)

# Generate POSTGRES_PASSWORD (32 characters)
echo "Generating POSTGRES_PASSWORD (32 chars)..."
POSTGRES_PASSWORD=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-32)

# Generate ENCRYPTION_KEY for backups (32 bytes = 64 hex chars)
echo "Generating ENCRYPTION_KEY for backups..."
ENCRYPTION_KEY=$(openssl rand -hex 32)

# Generate SIGNING_KEY for legal audit (64 bytes = 128 hex chars)
echo "Generating SIGNING_KEY for legal audit..."
SIGNING_KEY=$(openssl rand -hex 64)

# Create or update .env.production
cat > .env.production << EOF
# TuCentroPDF Engine V2 - Production Environment
# Generated on: $(date)
# KEEP THIS FILE SECURE - DO NOT COMMIT TO GIT

# ============================================================================
# CRITICAL SECRETS (GENERATED)
# ============================================================================
ENGINE_SECRET=${ENGINE_SECRET}
JWT_SECRET=${JWT_PRIVATE_KEY}
JWT_PUBLIC_KEY=${JWT_PUBLIC_KEY}
REDIS_PASSWORD=${REDIS_PASSWORD}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
SIGNING_KEY=${SIGNING_KEY}

# ============================================================================
# CORE ENGINE SETTINGS
# ============================================================================
ENVIRONMENT=production
ENGINE_PORT=8080
LOG_LEVEL=info
LOG_FORMAT=json

# ============================================================================
# AI/OCR CONFIGURATION
# ============================================================================
# OpenAI Configuration (REQUIRED for AI OCR)
AI_OCR_ENABLED=true
AI_PROVIDER=openai
AI_MODEL=gpt-4o-mini
OPENAI_API_KEY=your-openai-api-key-here

# Traditional OCR Configuration
OCR_PROVIDER=tesseract
OCR_LANGUAGES=eng,spa,por,fra

# ============================================================================
# OFFICE CONVERSION
# ============================================================================
OFFICE_ENABLED=true
OFFICE_PROVIDER=libreoffice

# ============================================================================
# DATABASE CONFIGURATION
# ============================================================================
# PostgreSQL (Optional - for analytics)
POSTGRES_ENABLED=false
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_USER=tucentropdf
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=tucentropdf_v2

# ============================================================================
# REDIS CONFIGURATION
# ============================================================================
REDIS_ENABLED=true
REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379/0
REDIS_PASSWORD=${REDIS_PASSWORD}
REDIS_MEMORY=256mb
REDIS_MEMORY_LIMIT=512M

# ============================================================================
# CORS CONFIGURATION
# ============================================================================
CORS_ORIGINS=https://tucentropdf.com,https://www.tucentropdf.com,https://api.tucentropdf.com,https://*.tucentropdf.com

# ============================================================================
# RESOURCE LIMITS
# ============================================================================
# Main Engine Resource Limits
ENGINE_MEMORY_LIMIT=2G
ENGINE_CPU_LIMIT=2
ENGINE_MEMORY_RESERVE=512M
ENGINE_CPU_RESERVE=0.5

# ============================================================================
# MONITORING (Optional)
# ============================================================================
# Grafana Admin Password
GF_SECURITY_ADMIN_PASSWORD=$(openssl rand -base64 24 | tr -d "=+/" | cut -c1-32)

# ============================================================================
# ALERTS (Optional)
# ============================================================================
ALERTS_EMAIL_ENABLED=false
ALERTS_SMTP_HOST=smtp.gmail.com
ALERTS_SMTP_PORT=587
ALERTS_SMTP_USER=your-email@gmail.com
ALERTS_SMTP_PASSWORD=your-app-password
ALERTS_SMTP_FROM=alerts@tucentropdf.com
ALERTS_SMTP_TO=admin@tucentropdf.com

ALERTS_TELEGRAM_ENABLED=false
ALERTS_TELEGRAM_BOT_TOKEN=your-bot-token
ALERTS_TELEGRAM_CHAT_ID=your-chat-id

# ============================================================================
# BACKUP CONFIGURATION (Optional)
# ============================================================================
BACKUP_ENABLED=false
BACKUP_ENCRYPTION_KEY=${ENCRYPTION_KEY}
BACKUP_RCLONE_REMOTE=s3:tucentropdf-backups
BACKUP_RETENTION_DAYS=30

# ============================================================================
# LEGAL AUDIT (Optional)
# ============================================================================
LEGAL_AUDIT_ENABLED=false
LEGAL_AUDIT_ENCRYPTION_KEY=${ENCRYPTION_KEY}
LEGAL_AUDIT_SIGNING_KEY=${SIGNING_KEY}
LEGAL_AUDIT_RETENTION_YEARS=7
EOF

echo ""
echo -e "${GREEN}✅ Secrets generated successfully!${NC}"
echo ""
echo "📝 Summary:"
echo "  - ENGINE_SECRET: 64 chars"
echo "  - JWT_SECRET: RSA 4096 bits"
echo "  - REDIS_PASSWORD: 32 chars"
echo "  - POSTGRES_PASSWORD: 32 chars"
echo "  - ENCRYPTION_KEY: 64 hex chars"
echo "  - SIGNING_KEY: 128 hex chars"
echo ""
echo -e "${YELLOW}⚠️  IMPORTANT:${NC}"
echo "  1. Update OPENAI_API_KEY in .env.production"
echo "  2. Update CORS_ORIGINS with your actual domains"
echo "  3. Configure SMTP settings if using email alerts"
echo "  4. Keep .env.production secure and NEVER commit to git"
echo "  5. Add .env.production to .gitignore"
echo ""
echo -e "${GREEN}Next steps:${NC}"
echo "  1. Review and update .env.production with your values"
echo "  2. Run: docker-compose --env-file .env.production up -d"
echo "  3. Verify: curl http://localhost:8080/health"
echo ""
