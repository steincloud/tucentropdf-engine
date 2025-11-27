#!/bin/bash
# monitor.sh - Script de monitoreo para TuCentroPDF Engine V2

# Variables
APP_DIR="/opt/tucentropdf/tucentropdf-engine/engine_v2"
LOG_FILE="/var/log/tucentropdf-monitor.log"

# Colores
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Función para mostrar header
show_header() {
    echo -e "${BLUE}================================================${NC}"
    echo -e "${BLUE}    TuCentroPDF Engine V2 - Monitor Status     ${NC}"
    echo -e "${BLUE}    $(date '+%Y-%m-%d %H:%M:%S')                ${NC}"
    echo -e "${BLUE}================================================${NC}"
}

# Función para verificar estado de contenedores
check_containers() {
    echo -e "\n${CYAN}🐳 Estado de Contenedores:${NC}"
    echo "=========================="
    
    cd "$APP_DIR" || exit 1
    
    # Estado de docker-compose
    docker-compose -f docker-compose.prod.yml ps
    
    # Verificar contenedores específicos
    local containers=("tucentropdf-engine" "tucentropdf-redis" "tucentropdf-nginx")
    
    echo -e "\n${CYAN}Detalles de contenedores:${NC}"
    for container in "${containers[@]}"; do
        if docker ps --format "table {{.Names}}" | grep -q "$container"; then
            echo -e "✅ $container: ${GREEN}RUNNING${NC}"
            
            # Obtener uso de recursos
            local stats
            stats=$(docker stats "$container" --no-stream --format "CPU: {{.CPUPerc}} | MEM: {{.MemUsage}} | NET: {{.NetIO}}")
            echo "   📊 $stats"
        else
            echo -e "❌ $container: ${RED}STOPPED${NC}"
        fi
    done
}

# Función para verificar recursos del sistema
check_system_resources() {
    echo -e "\n${CYAN}💻 Recursos del Sistema:${NC}"
    echo "========================"
    
    # CPU
    local cpu_usage
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)
    if (( $(echo "$cpu_usage > 80" | bc -l) )); then
        echo -e "🔥 CPU: ${RED}$cpu_usage%${NC} (ALTO)"
    elif (( $(echo "$cpu_usage > 50" | bc -l) )); then
        echo -e "⚠️ CPU: ${YELLOW}$cpu_usage%${NC} (MEDIO)"
    else
        echo -e "✅ CPU: ${GREEN}$cpu_usage%${NC} (NORMAL)"
    fi
    
    # Memoria
    local mem_info
    mem_info=$(free | grep Mem | awk '{printf "%.1f", $3/$2 * 100.0}')
    if (( $(echo "$mem_info > 85" | bc -l) )); then
        echo -e "🔥 RAM: ${RED}$mem_info%${NC} (ALTO)"
    elif (( $(echo "$mem_info > 70" | bc -l) )); then
        echo -e "⚠️ RAM: ${YELLOW}$mem_info%${NC} (MEDIO)"
    else
        echo -e "✅ RAM: ${GREEN}$mem_info%${NC} (NORMAL)"
    fi
    
    # Memoria detallada
    echo "📊 Memoria detallada:"
    free -h
    
    # Disco
    local disk_usage
    disk_usage=$(df -h / | awk 'NR==2{print $5}' | cut -d'%' -f1)
    if [ "$disk_usage" -gt 85 ]; then
        echo -e "🔥 Disco: ${RED}$disk_usage%${NC} (ALTO)"
    elif [ "$disk_usage" -gt 70 ]; then
        echo -e "⚠️ Disco: ${YELLOW}$disk_usage%${NC} (MEDIO)"
    else
        echo -e "✅ Disco: ${GREEN}$disk_usage%${NC} (NORMAL)"
    fi
    
    # Uptime
    echo "⏱️ Uptime: $(uptime -p)"
    
    # Load average
    echo "📈 Load: $(uptime | awk -F'load average:' '{print $2}')"
}

# Función para verificar estado de la API
check_api_health() {
    echo -e "\n${CYAN}🌐 Estado de la API:${NC}"
    echo "===================="
    
    local endpoints=(
        "http://localhost:8080/health"
        "http://localhost:8080/ready"
        "http://localhost:8080/metrics"
        "http://localhost:8080/api/v2/office/formats"
    )
    
    for endpoint in "${endpoints[@]}"; do
        local path
        path=$(echo "$endpoint" | sed 's|http://localhost:8080||')
        
        if curl -f -m 5 "$endpoint" > /dev/null 2>&1; then
            echo -e "✅ $path: ${GREEN}OK${NC}"
        else
            echo -e "❌ $path: ${RED}ERROR${NC}"
        fi
    done
    
    # Obtener métricas básicas si está disponible
    if curl -f -m 5 http://localhost:8080/metrics > /dev/null 2>&1; then
        echo -e "\n${CYAN}📊 Métricas básicas:${NC}"
        local metrics
        metrics=$(curl -s http://localhost:8080/metrics 2>/dev/null)
        
        # Requests totales
        echo "$metrics" | grep -E "requests_total" | head -3
        
        # Tiempo de respuesta
        echo "$metrics" | grep -E "response_time" | head -3
        
        # Errores
        echo "$metrics" | grep -E "errors_total" | head -3
    fi
}

# Función para verificar almacenamiento
check_storage() {
    echo -e "\n${CYAN}📁 Almacenamiento:${NC}"
    echo "=================="
    
    local dirs=("uploads" "temp" "logs")
    
    for dir in "${dirs[@]}"; do
        if [ -d "$dir" ]; then
            local size
            size=$(du -sh "$dir" 2>/dev/null | cut -f1)
            local files
            files=$(find "$dir" -type f 2>/dev/null | wc -l)
            echo "📂 $dir: $size ($files archivos)"
        else
            echo -e "❌ $dir: ${RED}NO EXISTE${NC}"
        fi
    done
    
    # Archivos temporales antiguos
    local old_temp
    old_temp=$(find temp/ -type f -mtime +1 2>/dev/null | wc -l)
    if [ "$old_temp" -gt 0 ]; then
        echo -e "⚠️ Archivos temporales antiguos: ${YELLOW}$old_temp${NC}"
    fi
    
    # Logs grandes
    local large_logs
    large_logs=$(find logs/ -name "*.log" -size +100M 2>/dev/null | wc -l)
    if [ "$large_logs" -gt 0 ]; then
        echo -e "⚠️ Logs grandes (>100MB): ${YELLOW}$large_logs${NC}"
    fi
}

# Función para verificar Redis
check_redis() {
    echo -e "\n${CYAN}📊 Estado de Redis:${NC}"
    echo "=================="
    
    if docker ps | grep -q tucentropdf-redis; then
        # Info básica
        local redis_info
        redis_info=$(docker exec tucentropdf-redis redis-cli info server 2>/dev/null | grep -E "(redis_version|uptime_in_seconds)")
        echo "$redis_info"
        
        # Memoria
        local redis_memory
        redis_memory=$(docker exec tucentropdf-redis redis-cli info memory 2>/dev/null | grep -E "(used_memory_human|used_memory_peak_human)")
        echo "$redis_memory"
        
        # Clientes conectados
        local redis_clients
        redis_clients=$(docker exec tucentropdf-redis redis-cli info clients 2>/dev/null | grep connected_clients)
        echo "$redis_clients"
        
        # Comandos por segundo
        local redis_stats
        redis_stats=$(docker exec tucentropdf-redis redis-cli info stats 2>/dev/null | grep instantaneous_ops_per_sec)
        echo "$redis_stats"
    else
        echo -e "❌ Redis: ${RED}NO DISPONIBLE${NC}"
    fi
}

# Función para mostrar logs recientes
show_recent_logs() {
    echo -e "\n${CYAN}📋 Logs Recientes:${NC}"
    echo "=================="
    
    echo -e "\n${YELLOW}Aplicación (últimas 10 líneas):${NC}"
    docker-compose -f docker-compose.prod.yml logs --tail=10 --timestamps tucentropdf-engine 2>/dev/null || echo "No hay logs disponibles"
    
    echo -e "\n${YELLOW}Redis (últimas 5 líneas):${NC}"
    docker-compose -f docker-compose.prod.yml logs --tail=5 --timestamps redis 2>/dev/null || echo "No hay logs disponibles"
    
    echo -e "\n${YELLOW}Nginx (últimas 5 líneas):${NC}"
    docker-compose -f docker-compose.prod.yml logs --tail=5 --timestamps nginx 2>/dev/null || echo "No hay logs disponibles"
}

# Función para verificar conectividad de red
check_network() {
    echo -e "\n${CYAN}🌐 Conectividad de Red:${NC}"
    echo "======================"
    
    # Verificar puertos
    local ports=("80:HTTP" "443:HTTPS" "8080:API" "6379:Redis")
    
    for port_info in "${ports[@]}"; do
        local port
        local name
        port=$(echo "$port_info" | cut -d':' -f1)
        name=$(echo "$port_info" | cut -d':' -f2)
        
        if netstat -tuln | grep -q ":$port "; then
            echo -e "✅ Puerto $port ($name): ${GREEN}ABIERTO${NC}"
        else
            echo -e "❌ Puerto $port ($name): ${RED}CERRADO${NC}"
        fi
    done
    
    # Test de conectividad externa
    if curl -f -m 5 https://api.openai.com > /dev/null 2>&1; then
        echo -e "✅ OpenAI API: ${GREEN}ACCESIBLE${NC}"
    else
        echo -e "❌ OpenAI API: ${RED}NO ACCESIBLE${NC}"
    fi
}

# Función para mostrar alertas
show_alerts() {
    echo -e "\n${CYAN}🚨 Alertas y Recomendaciones:${NC}"
    echo "============================="
    
    local alerts=()
    
    # CPU alto
    local cpu_usage
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)
    if (( $(echo "$cpu_usage > 80" | bc -l) )); then
        alerts+=("🔥 CPU alto ($cpu_usage%) - Considera optimizar o escalar")
    fi
    
    # RAM alta
    local mem_usage
    mem_usage=$(free | grep Mem | awk '{printf "%.1f", $3/$2 * 100.0}')
    if (( $(echo "$mem_usage > 85" | bc -l) )); then
        alerts+=("🔥 Memoria alta ($mem_usage%) - Revisa memory leaks")
    fi
    
    # Disco lleno
    local disk_usage
    disk_usage=$(df / | awk 'NR==2{print $5}' | cut -d'%' -f1)
    if [ "$disk_usage" -gt 85 ]; then
        alerts+=("🔥 Disco lleno ($disk_usage%) - Limpia archivos temporales")
    fi
    
    # Archivos temporales antiguos
    local old_files
    old_files=$(find temp/ -type f -mtime +1 2>/dev/null | wc -l)
    if [ "$old_files" -gt 100 ]; then
        alerts+=("⚠️ Muchos archivos temporales antiguos ($old_files)")
    fi
    
    # Logs grandes
    local large_logs
    large_logs=$(find logs/ -name "*.log" -size +100M 2>/dev/null)
    if [ -n "$large_logs" ]; then
        alerts+=("⚠️ Logs grandes detectados - considera rotación")
    fi
    
    # Contenedores parados
    if ! docker ps --format "{{.Names}}" | grep -q tucentropdf-engine; then
        alerts+=("🚨 Aplicación principal NO está corriendo")
    fi
    
    if ! docker ps --format "{{.Names}}" | grep -q tucentropdf-redis; then
        alerts+=("🚨 Redis NO está corriendo")
    fi
    
    # Mostrar alertas
    if [ ${#alerts[@]} -eq 0 ]; then
        echo -e "✅ ${GREEN}No hay alertas críticas${NC}"
    else
        for alert in "${alerts[@]}"; do
            echo "$alert"
        done
    fi
}

# Función para guardar reporte
save_report() {
    local report_file="/var/log/tucentropdf-status-$(date +%Y%m%d-%H%M%S).log"
    
    {
        show_header
        check_containers
        check_system_resources
        check_api_health
        check_storage
        check_redis
        check_network
        show_alerts
    } > "$report_file" 2>&1
    
    echo -e "\n📄 Reporte guardado: $report_file"
}

# Función de ayuda
show_help() {
    echo "Monitor de TuCentroPDF Engine V2"
    echo "Uso: $0 [opciones]"
    echo ""
    echo "Opciones:"
    echo "  -w, --watch     Monitor continuo (actualiza cada 30s)"
    echo "  -s, --save      Guardar reporte en archivo"
    echo "  -l, --logs      Mostrar solo logs recientes"
    echo "  -h, --help      Mostrar esta ayuda"
    echo ""
}

# Función principal
main() {
    case "${1:-}" in
        -w|--watch)
            echo "Iniciando monitor continuo (Ctrl+C para salir)..."
            while true; do
                clear
                show_header
                check_containers
                check_system_resources
                check_api_health
                show_alerts
                echo -e "\n${CYAN}⏱️ Actualizando en 30 segundos...${NC}"
                sleep 30
            done
            ;;
        -s|--save)
            save_report
            ;;
        -l|--logs)
            show_recent_logs
            ;;
        -h|--help)
            show_help
            ;;
        *)
            show_header
            check_containers
            check_system_resources
            check_api_health
            check_storage
            check_redis
            check_network
            show_recent_logs
            show_alerts
            ;;
    esac
}

# Ejecutar función principal con parámetros
main "$@"