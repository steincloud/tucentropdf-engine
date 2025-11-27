#!/bin/bash
# TuCentroPDF Engine V2 - Health Check Script
# Validates that all services are running properly

set -e

echo "🏥 TuCentroPDF Engine V2 - Health Check"
echo "========================================"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ERRORS=0
WARNINGS=0

# Function to check HTTP endpoint
check_endpoint() {
    local url=$1
    local name=$2
    local timeout=${3:-5}
    
    if curl -f -s --max-time "$timeout" "$url" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ $name is responding${NC}"
        return 0
    else
        echo -e "${RED}❌ $name is NOT responding${NC}"
        ((ERRORS++))
        return 1
    fi
}

# Function to check container
check_container() {
    local container=$1
    local name=$2
    
    if docker ps | grep -q "$container"; then
        local status=$(docker inspect --format='{{.State.Health.Status}}' "$container" 2>/dev/null || echo "unknown")
        if [ "$status" == "healthy" ] || [ "$status" == "unknown" ]; then
            echo -e "${GREEN}✅ $name container is running${NC}"
            return 0
        else
            echo -e "${YELLOW}⚠️  $name container status: $status${NC}"
            ((WARNINGS++))
            return 1
        fi
    else
        echo -e "${RED}❌ $name container is NOT running${NC}"
        ((ERRORS++))
        return 1
    fi
}

echo "🐳 Checking Docker containers..."
check_container "tucentropdf-engine" "Engine"
check_container "tucentropdf-redis" "Redis"
check_container "tucentropdf-nginx" "Nginx" || true

echo ""
echo "🌐 Checking HTTP endpoints..."
check_endpoint "http://localhost:8080/health/basic" "Engine Basic Health" 10
check_endpoint "http://localhost:8080/health" "Engine Full Health" 15
check_endpoint "http://localhost:8080/api/v1/info" "API Info" 10

echo ""
echo "💾 Checking Redis..."
if docker exec tucentropdf-redis redis-cli ping >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Redis is responding to PING${NC}"
else
    echo -e "${RED}❌ Redis is NOT responding${NC}"
    ((ERRORS++))
fi

echo ""
echo "📊 Checking resource usage..."

# Engine container stats
ENGINE_MEM=$(docker stats tucentropdf-engine --no-stream --format "{{.MemPerc}}" 2>/dev/null || echo "N/A")
ENGINE_CPU=$(docker stats tucentropdf-engine --no-stream --format "{{.CPUPerc}}" 2>/dev/null || echo "N/A")

echo "Engine Resources:"
echo "  Memory: $ENGINE_MEM"
echo "  CPU: $ENGINE_CPU"

# Redis container stats
REDIS_MEM=$(docker stats tucentropdf-redis --no-stream --format "{{.MemPerc}}" 2>/dev/null || echo "N/A")
REDIS_CPU=$(docker stats tucentropdf-redis --no-stream --format "{{.CPUPerc}}" 2>/dev/null || echo "N/A")

echo "Redis Resources:"
echo "  Memory: $REDIS_MEM"
echo "  CPU: $REDIS_CPU"

echo ""
echo "📝 Recent logs (last 10 lines):"
echo "Engine:"
docker logs tucentropdf-engine --tail 10 2>&1 | grep -E "(ERROR|WARN|✅|❌)" || echo "No recent errors or warnings"

echo ""
echo "========================================"
echo "Health Check Summary:"
echo "  Errors: $ERRORS"
echo "  Warnings: $WARNINGS"
echo "========================================"

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}❌ Health check FAILED${NC}"
    echo ""
    echo "Troubleshooting:"
    echo "  1. Check logs: docker-compose -f docker-compose.prod.yml logs -f"
    echo "  2. Restart services: docker-compose -f docker-compose.prod.yml restart"
    echo "  3. Check .env.production configuration"
    exit 1
fi

if [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Health check passed with warnings${NC}"
    exit 0
fi

echo -e "${GREEN}✅ All systems operational${NC}"
exit 0
