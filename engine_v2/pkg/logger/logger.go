package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.SugaredLogger
}

// New crea un nuevo logger estructurado
func New(level, format string, extras ...string) *Logger {
	var config zap.Config

	// Configurar nivel de log
	logLevel := zapcore.InfoLevel
	switch level {
	case "debug":
		logLevel = zapcore.DebugLevel
	case "info":
		logLevel = zapcore.InfoLevel
	case "warn":
		logLevel = zapcore.WarnLevel
	case "error":
		logLevel = zapcore.ErrorLevel
	}

	// Configurar formato
	switch format {
	case "json":
		config = zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(logLevel)
	default:
		config = zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(logLevel)
	}

	// Crear logger
	zapLogger, err := config.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		panic(err)
	}

	return &Logger{
		SugaredLogger: zapLogger.Sugar(),
	}
}

// WithFields añade campos al contexto del log
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	return &Logger{
		SugaredLogger: l.SugaredLogger.With(zapFieldsFromMap(fields)...),
	}
}

// WithRequest añade información de request
func (l *Logger) WithRequest(method, path, ip string) *Logger {
	return &Logger{
		SugaredLogger: l.SugaredLogger.With(
			"method", method,
			"path", path,
			"client_ip", ip,
		),
	}
}

// WithError añade información de error
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return &Logger{
		SugaredLogger: l.SugaredLogger.With("error", err.Error()),
	}
}

// zapFieldsFromMap convierte un map a campos zap
func zapFieldsFromMap(fields map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(fields)*2)
	for key, value := range fields {
		result = append(result, key, value)
	}
	return result
}

// NewLogger alias compatible con versiones previas que pasaban componente adicional
func NewLogger(level, format, _component string) *Logger {
	return New(level, format)
}