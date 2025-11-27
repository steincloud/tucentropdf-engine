# 🚀 TuCentroPDF Engine V2

## ⚠️ ARQUITECTURA EN DESARROLLO

Esta es la **nueva arquitectura** del motor TuCentroPDF, desarrollada en paralelo al motor original.

### 🎯 Objetivos V2

- ✅ API REST moderna con Go + Fiber
- ✅ Seguridad completa con autenticación
- ✅ Límites por plan (Free/Premium/Pro)
- ✅ Integración Office → PDF
- ✅ OCR clásico + IA (GPT-4.1-mini)
- ✅ Sistema de colas y storage
- ✅ Logging estructurado JSON
- ✅ Docker production-ready

### 🏗️ Arquitectura

```
engine_v2/
├── cmd/server/           # Servidor principal
├── internal/
│   ├── api/             # REST API handlers
│   ├── pdf/             # Core PDF operations
│   ├── office/          # Office conversion
│   ├── ocr/             # OCR services
│   ├── limits/          # Plan limits
│   ├── security/        # Auth & security
│   └── storage/         # File management
├── pkg/                 # Shared packages
└── docs/                # Documentation
```

### 🔄 Estado de Desarrollo

- [x] **Fase 1:** Arquitectura base ← **ACTUAL**
- [ ] **Fase 2:** Integración Office + OCR
- [ ] **Fase 3:** IA + Límites avanzados
- [ ] **Fase 4:** Testing + Docker
- [ ] **Fase 5:** Migración final

### ⚠️ IMPORTANTE

**NO reemplaza el motor original** hasta aprobación final.
Motor original en raíz sigue siendo la versión de producción actual.

---
**Desarrollado con extremo cuidado para TuCentroPDF** 🚀