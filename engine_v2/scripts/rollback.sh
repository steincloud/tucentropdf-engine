#!/bin/bash
# TuCentroPDF Engine V2 - Rollback Script
# Rolls back to previous deployment

set -e

echo "🔄 TuCentroPDF Engine V2 - Rollback"
echo "===================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

COMPOSE_FILE="docker-compose.prod.yml"
ENV_FILE=".env.production"
BACKUP_DIR="./backups"

# Check if backups exist
if [ ! -d "$BACKUP_DIR" ]; then
    echo -e "${RED}❌ No backups directory found${NC}"
    exit 1
fi

# List available backups
echo "Available backups:"
ls -lh "$BACKUP_DIR"/backup-*.tar.gz 2>/dev/null || {
    echo -e "${RED}❌ No backups found${NC}"
    exit 1
}

# Get the latest backup
LATEST_BACKUP=$(ls -t "$BACKUP_DIR"/backup-*.tar.gz 2>/dev/null | head -1)

if [ -z "$LATEST_BACKUP" ]; then
    echo -e "${RED}❌ No backup files found${NC}"
    exit 1
fi

echo ""
echo -e "${YELLOW}Latest backup: $LATEST_BACKUP${NC}"
read -p "Rollback to this version? (y/N): " -n 1 -r
echo

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Rollback cancelled"
    exit 0
fi

echo ""
echo "🛑 Stopping current containers..."
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" down

echo ""
echo "📦 Restoring backup..."
tar -xzf "$LATEST_BACKUP" -C .

echo ""
echo "🚀 Starting containers with restored configuration..."
docker-compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d

echo ""
echo "⏳ Waiting for services..."
sleep 10

# Health check
if curl -f -s http://localhost:8080/health >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Rollback successful${NC}"
else
    echo -e "${RED}❌ Rollback may have issues, check logs${NC}"
    echo "docker-compose -f $COMPOSE_FILE logs -f"
fi

echo ""
echo "===================================="
echo -e "${GREEN}Rollback completed${NC}"
echo "===================================="
echo ""
