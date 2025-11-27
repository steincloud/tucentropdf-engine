# 📚 Documentación TuCentroPDF Engine V2

## 📖 Índice General

### 🚀 Inicio Rápido
- **[QUICK_DEPLOY.md](QUICK_DEPLOY.md)** - Guía de implementación en VPS en 5 pasos
- **[README.md](README.md)** - Introducción y características principales

### 🏗️ Desarrollo y Arquitectura
- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** - Arquitectura del sistema y componentes
- **[docs/API.md](docs/API.md)** - Documentación completa de la API REST

### 🧪 Testing y Calidad
- **[docs/TESTING.md](docs/TESTING.md)** - Guía completa de testing automatizado
- **[PHASE2_COMPLETED.md](PHASE2_COMPLETED.md)** - Fase 2: Conversión Office y funcionalidades avanzadas
- **[FASE3_COMPLETED.md](FASE3_COMPLETED.md)** - Fase 3: OCR con IA y límites de plan

### 🚀 Despliegue y Producción
- **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** - Guía completa de despliegue en VPS
- **[DEPLOY.md](DEPLOY.md)** - Configuración Docker y producción

### 📋 Configuración de Fases
- **[FASE1_COMPLETED.md](FASE1_COMPLETED.md)** - Fase 1: API base y operaciones PDF básicas

---

## 🛠️ Scripts de Automatización

### 📁 `scripts/`
- **[setup.sh](scripts/setup.sh)** - Configuración inicial completa del VPS
- **[deploy.sh](scripts/deploy.sh)** - Script de despliegue automático
- **[update.sh](scripts/update.sh)** - Actualizaciones sin downtime
- **[monitor.sh](scripts/monitor.sh)** - Monitoreo del sistema
- **[backup.sh](scripts/backup.sh)** - Backups automáticos

---

## 🔧 Configuración

### 📁 `config/`
- **[docker-compose.yml](docker-compose.yml)** - Desarrollo local
- **[docker-compose.prod.yml](docker-compose.prod.yml)** - Producción completa
- **[Dockerfile](Dockerfile)** - Imagen de producción optimizada

### 📁 `.github/workflows/`
- **[ci.yml](.github/workflows/ci.yml)** - Pipeline CI/CD completo

---

## 🏃‍♂️ Guías de Uso Rápido

### Para Desarrolladores
```bash
# Configurar entorno de desarrollo
1. Leer: docs/ARCHITECTURE.md
2. Leer: docs/TESTING.md
3. Ejecutar: docker-compose up -d
4. Correr tests: make test
```

### Para DevOps/Administradores
```bash
# Desplegar en VPS
1. Leer: QUICK_DEPLOY.md (5 minutos)
2. Ejecutar: ./scripts/setup.sh (configuración VPS)
3. Ejecutar: ./scripts/deploy.sh (despliegue)
4. Leer: docs/DEPLOYMENT.md (configuración avanzada)
```

### Para Integradores de Frontend
```bash
# Integrar con aplicación web
1. Leer: docs/API.md (endpoints)
2. Usar: JavaScript client en QUICK_DEPLOY.md
3. Ver ejemplos: docs/API.md sección Examples
```

---

## 📊 Estado del Proyecto

### ✅ Fases Completadas

#### Fase 1: API Base ✅
- API REST completa
- Operaciones PDF básicas
- Validación y manejo de errores
- Documentación: [FASE1_COMPLETED.md](FASE1_COMPLETED.md)

#### Fase 2: Funcionalidades Avanzadas ✅
- Conversión de documentos Office
- Operaciones avanzadas PDF
- Optimización de rendimiento
- Documentación: [PHASE2_COMPLETED.md](PHASE2_COMPLETED.md)

#### Fase 3: OCR con IA ✅
- OCR con Tesseract optimizado
- Sistema de límites por plan
- Manejo de múltiples idiomas
- Documentación: [FASE3_COMPLETED.md](FASE3_COMPLETED.md)

#### Fase 4: Testing Completo ✅
- Suite de testing automatizado
- CI/CD con GitHub Actions
- Testing de integración
- Documentación: [docs/TESTING.md](docs/TESTING.md)

#### Fase 5: Producción VPS ✅
- Despliegue automático completo
- Monitoreo y backups
- SSL y seguridad
- Documentación: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) + [QUICK_DEPLOY.md](QUICK_DEPLOY.md)

---

## 🔄 Flujo de Trabajo Recomendado

### Desarrollo Local
```bash
1. git clone <repositorio>
2. cd engine_v2
3. cp .env.example .env
4. docker-compose up -d
5. make test
6. make run
```

### Testing
```bash
1. make test          # Tests unitarios
2. make test-coverage # Cobertura de tests
3. make lint          # Linting
4. make integration   # Tests de integración
```

### Despliegue
```bash
1. ./scripts/setup.sh    # Solo primera vez en VPS
2. ./scripts/deploy.sh   # Despliegue automático
3. ./scripts/monitor.sh  # Monitoreo post-despliegue
```

### Mantenimiento
```bash
1. ./scripts/update.sh   # Actualizaciones
2. ./scripts/backup.sh   # Backups manuales
3. ./scripts/monitor.sh  # Estado del sistema
```

---

## 🆘 Solución de Problemas

### Problemas Comunes
1. **Error de conexión**: Revisar [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) sección troubleshooting
2. **Tests fallando**: Ver [docs/TESTING.md](docs/TESTING.md) sección debugging
3. **Performance lenta**: Consultar [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) optimización

### Logs Importantes
- Aplicación: `/var/log/tucentropdf/`
- Docker: `docker logs tucentropdf-engine`
- Nginx: `/var/log/nginx/`
- Sistema: `journalctl -u tucentropdf-engine`

### Comandos de Diagnóstico
```bash
# Estado general
tucentropdf-status

# Verificar servicios
systemctl status nginx docker
docker ps -a

# Verificar recursos
htop
df -h
free -h

# Logs en tiempo real
tail -f /var/log/tucentropdf/application.log
```

---

## 📈 Roadmap y Mejoras Futuras

### Próximas Funcionalidades
- [ ] Dashboard de administración web
- [ ] API de webhooks para notificaciones
- [ ] Procesamiento en batch
- [ ] Cache distribuido con Redis Cluster
- [ ] Métricas avanzadas con Prometheus
- [ ] Autoscaling con Kubernetes

### Optimizaciones Pendientes
- [ ] Compresión de imágenes optimizada
- [ ] Workers distribuidos
- [ ] CDN para archivos estáticos
- [ ] Base de datos para metadata

---

## 🤝 Contribución

### Para Contribuir
1. Fork del repositorio
2. Crear rama feature: `git checkout -b feature/nueva-funcionalidad`
3. Seguir guías en [docs/TESTING.md](docs/TESTING.md)
4. Crear PR con tests incluidos

### Estándares de Código
- Seguir patrones en [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- Tests obligatorios para nueva funcionalidad
- Documentación actualizada en [docs/API.md](docs/API.md)

---

## 📞 Soporte

### Documentación
- **Técnica**: docs/
- **Usuario final**: QUICK_DEPLOY.md
- **API**: docs/API.md

### Contacto
- Issues: GitHub Issues
- Documentación: Este repositorio
- Ejemplos: docs/API.md

---

**🎉 TuCentroPDF Engine V2 - Listo para producción**