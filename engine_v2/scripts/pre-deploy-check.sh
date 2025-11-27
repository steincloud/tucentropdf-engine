#!/bin/bash
# TuCentroPDF Engine V2 - Pre-Deploy Validation Script
# Validates configuration and environment before deployment

set -e

echo "🔍 Running pre-deploy validation checks..."
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

# Function to check critical config
check_critical() {
    local key=$1
    local value=$2
    local name=$3
    
    if [ -z "$value" ] || [ "$value" == "CHANGE-THIS"* ] || [ "$value" == "your-"* ]; then
        echo -e "${RED}❌ $name not configured${NC}"
        echo "   Please set $key in .env.production"
        ((ERRORS++))
        return 1
    fi
    echo -e "${GREEN}✅ $name configured${NC}"
    return 0
}

# Function to check optional config
check_optional() {
    local key=$1
    local value=$2
    local name=$3
    
    if [ -z "$value" ] || [ "$value" == "your-"* ]; then
        echo -e "${YELLOW}⚠️  $name not configured (optional)${NC}"
        ((WARNINGS++))
        return 1
    fi
    echo -e "${GREEN}✅ $name configured${NC}"
    return 0
}

# Check if .env.production exists
if [ ! -f .env.production ]; then
    echo -e "${RED}❌ .env.production not found${NC}"
    echo "   Run: ./scripts/generate-secrets.sh"
    exit 1
fi

# Load environment
export $(cat .env.production | grep -v '^#' | xargs)

echo "📝 Checking critical secrets..."
check_critical "ENGINE_SECRET" "$ENGINE_SECRET" "Engine Secret"
check_critical "JWT_SECRET" "$JWT_SECRET" "JWT Secret"
check_critical "REDIS_PASSWORD" "$REDIS_PASSWORD" "Redis Password"

# Validate ENGINE_SECRET length
if [ ${#ENGINE_SECRET} -lt 32 ]; then
    echo -e "${RED}❌ ENGINE_SECRET must be at least 32 characters${NC}"
    ((ERRORS++))
fi

echo ""
echo "🔑 Checking API configuration..."
check_optional "OPENAI_API_KEY" "$OPENAI_API_KEY" "OpenAI API Key"

echo ""
echo "🌐 Checking CORS configuration..."
if [ -z "$CORS_ORIGINS" ]; then
    echo -e "${YELLOW}⚠️  CORS_ORIGINS not set, using defaults${NC}"
    ((WARNINGS++))
else
    echo -e "${GREEN}✅ CORS Origins: $CORS_ORIGINS${NC}"
fi

echo ""
echo "💾 Checking directories..."

# Check temp directory
if [ ! -d "./temp" ]; then
    echo -e "${YELLOW}⚠️  Creating temp directory${NC}"
    mkdir -p ./temp
fi

# Check uploads directory
if [ ! -d "./uploads" ]; then
    echo -e "${YELLOW}⚠️  Creating uploads directory${NC}"
    mkdir -p ./uploads
fi

# Check logs directory
if [ ! -d "./logs" ]; then
    echo -e "${YELLOW}⚠️  Creating logs directory${NC}"
    mkdir -p ./logs
fi

echo ""
echo "🐳 Checking Docker..."
if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Docker not installed${NC}"
    ((ERRORS++))
else
    echo -e "${GREEN}✅ Docker installed: $(docker --version)${NC}"
fi

if ! command -v docker-compose &> /dev/null; then
    echo -e "${RED}❌ Docker Compose not installed${NC}"
    ((ERRORS++))
else
    echo -e "${GREEN}✅ Docker Compose installed: $(docker-compose --version)${NC}"
fi

echo ""
echo "📦 Checking Nginx configuration..."
if [ ! -f "./nginx/nginx.conf" ]; then
    echo -e "${RED}❌ nginx/nginx.conf not found${NC}"
    ((ERRORS++))
else
    echo -e "${GREEN}✅ Nginx config found${NC}"
fi

if [ ! -f "./nginx/sites-available/tucentropdf.conf" ]; then
    echo -e "${RED}❌ nginx/sites-available/tucentropdf.conf not found${NC}"
    ((ERRORS++))
else
    echo -e "${GREEN}✅ Nginx site config found${NC}"
fi

echo ""
echo "🔧 Validating Go code..."
if command -v go &> /dev/null; then
    echo "Running go vet..."
    if go vet ./...; then
        echo -e "${GREEN}✅ Go code validation passed${NC}"
    else
        echo -e "${YELLOW}⚠️  Go vet found issues${NC}"
        ((WARNINGS++))
    fi
else
    echo -e "${YELLOW}⚠️  Go not installed, skipping code validation${NC}"
    ((WARNINGS++))
fi

echo ""
echo "=================================================="
echo "Validation Summary:"
echo "  Errors: $ERRORS"
echo "  Warnings: $WARNINGS"
echo "=================================================="

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}❌ Pre-deploy validation FAILED${NC}"
    echo "Please fix the errors above before deploying"
    exit 1
fi

if [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Pre-deploy validation passed with warnings${NC}"
    echo "Review the warnings above"
else
    echo -e "${GREEN}✅ Pre-deploy validation PASSED${NC}"
fi

echo ""
echo "Next steps:"
echo "  1. Review docker-compose.prod.yml"
echo "  2. Run: ./scripts/deploy-vps.sh"
echo ""
