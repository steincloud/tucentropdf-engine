package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"time"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service servicio de alertas internas
type Service struct {
	config *config.Config
	logger *logger.Logger
	
	// Configuración de alertas
	emailEnabled    bool
	telegramEnabled bool
	smtpConfig      *SMTPConfig
	telegramConfig  *TelegramConfig
}

// Alert estructura de alerta
type Alert struct {
	Type      string                 `json:"type"`
	Severity  string                 `json:"severity"` // info, warning, critical
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
	Timestamp time.Time              `json:"timestamp"`
}

// SMTPConfig configuración de SMTP
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

// TelegramConfig configuración de Telegram
type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// NewService crea nuevo servicio de alertas
func NewService(cfg *config.Config, log *logger.Logger) *Service {
	return &Service{
		config: cfg,
		logger: log,
		// Configuración desde variables de entorno o config
		emailEnabled:    cfg.Alerts.EmailEnabled,
		telegramEnabled: cfg.Alerts.TelegramEnabled,
		smtpConfig:      getSMTPConfig(cfg),
		telegramConfig:  getTelegramConfig(cfg),
	}
}

// SendAlert envía una alerta usando todos los canales configurados
func (s *Service) SendAlert(alert *Alert) {
	alert.Timestamp = time.Now()
	
	// Log la alerta siempre
	s.logAlert(alert)
	
	// Enviar por email si está habilitado
	if s.emailEnabled && s.smtpConfig != nil {
		go s.sendEmailAlert(alert)
	}
	
	// Enviar por Telegram si está habilitado
	if s.telegramEnabled && s.telegramConfig != nil {
		go s.sendTelegramAlert(alert)
	}
}

// logAlert registra la alerta en logs
func (s *Service) logAlert(alert *Alert) {
	message := fmt.Sprintf("🚨 ALERT [%s]: %s", alert.Type, alert.Message)
	
	switch alert.Severity {
	case "critical":
		s.logger.Error(message, "type", alert.Type, "details", alert.Details)
	case "warning":
		s.logger.Warn(message, "type", alert.Type, "details", alert.Details)
	default:
		s.logger.Info(message, "type", alert.Type, "details", alert.Details)
	}
}

// sendEmailAlert envía alerta por email
func (s *Service) sendEmailAlert(alert *Alert) {
	if s.smtpConfig == nil {
		return
	}
	
	subject := fmt.Sprintf("[TuCentroPDF] %s Alert: %s", alert.Severity, alert.Type)
	body := s.formatEmailBody(alert)
	
	// Configurar autenticación SMTP
	auth := smtp.PlainAuth("", s.smtpConfig.Username, s.smtpConfig.Password, s.smtpConfig.Host)
	
	// Crear mensaje
	msg := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s",
		s.smtpConfig.To[0], subject, body)
	
	// Enviar email
	addr := fmt.Sprintf("%s:%d", s.smtpConfig.Host, s.smtpConfig.Port)
	err := smtp.SendMail(addr, auth, s.smtpConfig.From, s.smtpConfig.To, []byte(msg))
	
	if err != nil {
		s.logger.Error("Failed to send email alert", "error", err, "type", alert.Type)
	} else {
		s.logger.Debug("Email alert sent", "type", alert.Type, "to", s.smtpConfig.To[0])
	}
}

// sendTelegramAlert envía alerta por Telegram
func (s *Service) sendTelegramAlert(alert *Alert) {
	if s.telegramConfig == nil {
		return
	}
	
	message := s.formatTelegramMessage(alert)
	
	// URL de la API de Telegram
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.telegramConfig.BotToken)
	
	// Payload
	payload := map[string]interface{}{
		"chat_id":    s.telegramConfig.ChatID,
		"text":       message,
		"parse_mode": "HTML",
	}
	
	jsonPayload, _ := json.Marshal(payload)
	
	// Enviar request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		s.logger.Error("Failed to send Telegram alert", "error", err, "type", alert.Type)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		s.logger.Debug("Telegram alert sent", "type", alert.Type)
	} else {
		s.logger.Error("Telegram API error", "status", resp.StatusCode, "type", alert.Type)
	}
}

// formatEmailBody formatea el cuerpo del email
func (s *Service) formatEmailBody(alert *Alert) string {
	body := fmt.Sprintf(`TuCentroPDF Engine Alert

Type: %s
Severity: %s
Time: %s
Message: %s

`,
		alert.Type, alert.Severity, alert.Timestamp.Format("2006-01-02 15:04:05"), alert.Message)
	
	if alert.Details != nil {
		body += "Details:\n"
		for key, value := range alert.Details {
			body += fmt.Sprintf("  %s: %v\n", key, value)
		}
	}
	
	body += "\n--\nTuCentroPDF Engine V2\nInternal Monitoring System"
	
	return body
}

// formatTelegramMessage formatea el mensaje de Telegram
func (s *Service) formatTelegramMessage(alert *Alert) string {
	var emoji string
	switch alert.Severity {
	case "critical":
		emoji = "🚨"
	case "warning":
		emoji = "⚠️"
	default:
		emoji = "ℹ️"
	}
	
	message := fmt.Sprintf("%s <b>%s Alert</b>\n", emoji, alert.Severity)
	message += fmt.Sprintf("<b>Type:</b> %s\n", alert.Type)
	message += fmt.Sprintf("<b>Message:</b> %s\n", alert.Message)
	message += fmt.Sprintf("<b>Time:</b> %s\n", alert.Timestamp.Format("15:04:05"))
	
	if alert.Details != nil {
		message += "\n<b>Details:</b>\n"
		for key, value := range alert.Details {
			message += fmt.Sprintf("• %s: %v\n", key, value)
		}
	}
	
	return message
}

// getSMTPConfig obtiene configuración SMTP
func getSMTPConfig(cfg *config.Config) *SMTPConfig {
	if !cfg.Alerts.EmailEnabled {
		return nil
	}
	
	return &SMTPConfig{
		Host:     cfg.Alerts.SMTPHost,
		Port:     cfg.Alerts.SMTPPort,
		Username: cfg.Alerts.SMTPUsername,
		Password: cfg.Alerts.SMTPPassword,
		From:     cfg.Alerts.EmailFrom,
		To:       cfg.Alerts.EmailTo,
	}
}

// getTelegramConfig obtiene configuración Telegram
func getTelegramConfig(cfg *config.Config) *TelegramConfig {
	if !cfg.Alerts.TelegramEnabled {
		return nil
	}
	
	return &TelegramConfig{
		BotToken: cfg.Alerts.TelegramBotToken,
		ChatID:   cfg.Alerts.TelegramChatID,
	}
}