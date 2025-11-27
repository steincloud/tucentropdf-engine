#!/bin/bash
# TuCentroPDF Engine V2 - VPS Deployment Script
# Deploys the complete stack to production VPS

set -e

echo "🚀 TuCentroPDF Engine V2 - VPS Deployment"
echo "=========================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="docker-compose.prod.yml"
ENV_FILE=".env.production"
BACKUP_DIR="./backups"

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then 
    echo -e "${YELLOW}⚠️  Not running as root. Some operations may require sudo.${NC}"
fi

# Run pre-deploy checks
echo -e "${BLUE}Step 1: Running pre-deploy validation...${NC}"
if [ -f "./scripts/pre-deploy-check.sh" ]; then
    bash ./scripts/pre-deploy-check.sh
    if [ $? -ne 0 ]; then
        echo -e "${RED}❌ Pre-deploy validation failed${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}⚠️  pre-deploy-check.sh not found, skipping validation${NC}"
fi

echo ""
echo -e "${BLUE}Step 2: Creating backup of current state...${NC}"
mkdir -p "$BACKUP_DIR"
BACKUP_FILE="$BACKUP_DIR/backup-$(date +%Y%m%d-%H%M%S).tar.gz"

if docker-compose -f "$COMPOSE_FILE" ps | grep -q "Up"; then
    echo "Creating backup..."
    tar -czf "$BACKUP_FILE" \
        --exclude='./temp/*' \
        --exclude='./logs/*' \
        --exclude='./backups/*' \
        .env.production docker-compose.prod.yml 2>/dev/null || true
    echo -e "${GREEN}✅ Backup created: $BACKUP_FILE${NC}"
else
    echo -e "${YELLOW}⚠️  No running containers, skipping backup${NC}"
fi

echo ""
echo -e "${BLUE}Step 3: Pulling latest Docker images...${NC}"
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" pull

echo ""
echo -e "${BLUE}Step 4: Building application image...${NC}"
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" build --no-cache

echo ""
echo -e "${BLUE}Step 5: Stopping old containers...${NC}"
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" down

echo ""
echo -e "${BLUE}Step 6: Starting new containers...${NC}"
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d

echo ""
echo -e "${BLUE}Step 7: Waiting for services to be ready...${NC}"
sleep 10

# Health check
echo ""
echo -e "${BLUE}Step 8: Running health checks...${NC}"
MAX_RETRIES=12
RETRY=0

while [ $RETRY -lt $MAX_RETRIES ]; do
    if curl -f -s http://localhost:8080/health >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Engine is healthy${NC}"
        break
    fi
    
    RETRY=$((RETRY + 1))
    echo "Waiting for engine to be ready... ($RETRY/$MAX_RETRIES)"
    sleep 5
done

if [ $RETRY -eq $MAX_RETRIES ]; then
    echo -e "${RED}❌ Engine failed to start properly${NC}"
    echo "Check logs with: docker-compose -f $COMPOSE_FILE logs engine"
    echo ""
    echo "Rolling back..."
    docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" down
    exit 1
fi

# Check Redis
if docker-compose -f "$COMPOSE_FILE" exec -T redis redis-cli -a "$REDIS_PASSWORD" ping >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Redis is healthy${NC}"
else
    echo -e "${YELLOW}⚠️  Redis health check failed${NC}"
fi

echo ""
echo -e "${BLUE}Step 9: Cleaning up old images...${NC}"
docker image prune -f

echo ""
echo "=========================================="
echo -e "${GREEN}✅ Deployment completed successfully!${NC}"
echo "=========================================="
echo ""
echo "📊 Service Status:"
docker-compose -f "$COMPOSE_FILE" ps
echo ""
echo "🔗 Access your API:"
echo "  http://localhost:8080/health"
echo "  http://localhost:8080/api/v1/info"
echo ""
echo "📝 View logs:"
echo "  docker-compose -f $COMPOSE_FILE logs -f engine"
echo ""
echo "🛑 Stop services:"
echo "  docker-compose -f $COMPOSE_FILE down"
echo ""
echo "🔄 Rollback if needed:"
echo "  ./scripts/rollback.sh"
echo ""
