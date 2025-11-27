package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/audit"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// UserManager servicio de gestión de usuarios
type UserManager struct {
	redis           *redis.Client
	logger          *logger.Logger
	config          *config.Config
	auditLogger     audit.AuditLogger
	webhookManager  *WebhookEventManager
	prorationService *config.ProrationService
}

// NewUserManager crea un nuevo gestor de usuarios
func NewUserManager(
	redisClient *redis.Client,
	log *logger.Logger,
	cfg *config.Config,
	auditLogger audit.AuditLogger,
	webhookManager *WebhookEventManager,
) *UserManager {
	return &UserManager{
		redis:           redisClient,
		logger:          log,
		config:          cfg,
		auditLogger:     auditLogger,
		webhookManager:  webhookManager,
		prorationService: config.NewProrationService(),
	}
}

// CreateUser crea un nuevo usuario
func (um *UserManager) CreateUser(ctx context.Context, user *config.User) error {
	// Validar datos del usuario
	if err := user.Validate(); err != nil {
		return fmt.Errorf("invalid user data: %w", err)
	}
	
	// Verificar que el usuario no exista
	exists, err := um.UserExists(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	
	if exists {
		return fmt.Errorf("user already exists: %s", user.ID)
	}
	
	// Establecer timestamps
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	
	// Serializar usuario
	userData, err := user.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user: %w", err)
	}
	
	// Guardar en Redis
	pipe := um.redis.Pipeline()
	
	// Guardar datos completos del usuario
	pipe.Set(ctx, um.keyUserData(user.ID), userData, 0) // Sin expiración
	
	// Guardar plan del usuario para acceso rápido
	pipe.Set(ctx, um.keyUserPlan(user.ID), string(user.Plan), 0)
	
	// Agregar a índice de usuarios por plan
	pipe.SAdd(ctx, um.keyUsersByPlan(user.Plan), user.ID)
	
	// Agregar a índice general de usuarios
	pipe.SAdd(ctx, um.keyAllUsers(), user.ID)
	
	// Guardar timestamp de creación
	pipe.Set(ctx, um.keyUserCreatedAt(user.ID), now.Unix(), 0)
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	
	// Log de auditoría
	um.auditLogger.LogEvent(audit.AuditEvent{
		EventType: audit.EventPlanChanged,
		UserID:    user.ID,
		Data: map[string]interface{}{
			"action":      "user_created",
			"plan":        user.Plan,
			"status":      user.Status,
			"created_at":  user.CreatedAt,
		},
		Timestamp: now,
	})
	
	// Crear evento de webhook
	webhookEvent := &WebhookEvent{
		Type:   WebhookUserCreated,
		UserID: user.ID,
		Data: map[string]interface{}{
			"user_id":    user.ID,
			"email":      user.Email,
			"name":       user.Name,
			"plan":       user.Plan,
			"status":     user.Status,
			"created_at": user.CreatedAt.Format(time.RFC3339),
		},
	}
	um.webhookManager.QueueEvent(ctx, webhookEvent)
	
	um.logger.Info("User created successfully",
		"user_id", user.ID,
		"plan", user.Plan,
		"email", user.Email,
	)
	
	return nil
}

// GetUser obtiene un usuario por ID
func (um *UserManager) GetUser(ctx context.Context, userID string) (*config.User, error) {
	userData, err := um.redis.Get(ctx, um.keyUserData(userID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	
	var user config.User
	if err := user.FromJSON([]byte(userData)); err != nil {
		return nil, fmt.Errorf("failed to deserialize user: %w", err)
	}
	
	return &user, nil
}

// UpdateUser actualiza un usuario existente
func (um *UserManager) UpdateUser(ctx context.Context, user *config.User) error {
	// Validar datos del usuario
	if err := user.Validate(); err != nil {
		return fmt.Errorf("invalid user data: %w", err)
	}
	
	// Verificar que el usuario exista
	exists, err := um.UserExists(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	
	if !exists {
		return fmt.Errorf("user not found: %s", user.ID)
	}
	
	// Obtener usuario actual para comparación
	currentUser, err := um.GetUser(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	
	// Actualizar timestamp
	user.UpdatedAt = time.Now()
	
	// Serializar usuario actualizado
	userData, err := user.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user: %w", err)
	}
	
	// Guardar en Redis
	pipe := um.redis.Pipeline()
	
	// Actualizar datos del usuario
	pipe.Set(ctx, um.keyUserData(user.ID), userData, 0)
	pipe.Set(ctx, um.keyUserPlan(user.ID), string(user.Plan), 0)
	
	// Si cambió el plan, actualizar índices
	if currentUser.Plan != user.Plan {
		pipe.SRem(ctx, um.keyUsersByPlan(currentUser.Plan), user.ID)
		pipe.SAdd(ctx, um.keyUsersByPlan(user.Plan), user.ID)
	}
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	
	// Log de auditoría para cambios importantes
	changes := um.detectUserChanges(currentUser, user)
	if len(changes) > 0 {
		um.auditLogger.LogEvent(audit.AuditEvent{
			EventType: audit.EventPlanChanged,
			UserID:    user.ID,
			Data: map[string]interface{}{
				"action":     "user_updated",
				"changes":    changes,
				"updated_at": user.UpdatedAt,
			},
			Timestamp: user.UpdatedAt,
		})
		
		// Crear evento de webhook si hay cambios significativos
		if um.hasSignificantChanges(changes) {
			webhookEvent := &WebhookEvent{
				Type:   WebhookUserUpdated,
				UserID: user.ID,
				Data: map[string]interface{}{
					"user_id":    user.ID,
					"changes":    changes,
					"updated_at": user.UpdatedAt.Format(time.RFC3339),
				},
			}
			um.webhookManager.QueueEvent(ctx, webhookEvent)
		}
	}
	
	um.logger.Info("User updated successfully",
		"user_id", user.ID,
		"changes", len(changes),
	)
	
	return nil
}

// ChangePlan cambia el plan de un usuario con prorrateo
func (um *UserManager) ChangePlan(ctx context.Context, userID string, newPlan config.Plan, billingCycle config.BillingCycle) (*config.ProrationCalculation, error) {
	// Obtener usuario actual
	user, err := um.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	
	// Verificar que puede cambiar al nuevo plan
	if !user.CanUpgradeToPlan(newPlan) {
		return nil, fmt.Errorf("cannot change to plan %s from %s", newPlan, user.Plan)
	}
	
	// Calcular prorrateo
	var prorationCalc *config.ProrationCalculation
	if user.Plan != config.PlanFree && newPlan != config.PlanFree {
		// Solo calcular prorrateo si ambos planes son de pago
		lastPayment := time.Now()
		if user.LastPayment != nil {
			lastPayment = *user.LastPayment
		}
		
		// Calcular duración del ciclo
		var cycleDuration time.Duration
		if billingCycle == config.BillingMonthly {
			cycleDuration = 30 * 24 * time.Hour // Aproximadamente un mes
		} else {
			cycleDuration = 365 * 24 * time.Hour // Un año
		}
		
		timeUsed := time.Since(lastPayment)
		if timeUsed > cycleDuration {
			timeUsed = cycleDuration // No puede exceder la duración del ciclo
		}
		
		prorationCalc, err = um.prorationService.CalcularProrrateo(
			user.Plan,
			newPlan,
			billingCycle,
			timeUsed,
			cycleDuration,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate proration: %w", err)
		}
	}
	
	// Actualizar plan del usuario
	oldPlan := user.Plan
	user.Plan = newPlan
	user.BillingCycle = billingCycle
	user.UpdatedAt = time.Now()
	
	// Guardar usuario actualizado
	if err := um.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user plan: %w", err)
	}
	
	// Log de auditoría
	auditData := map[string]interface{}{
		"action":    "plan_changed",
		"old_plan":  oldPlan,
		"new_plan":  newPlan,
		"is_upgrade": user.Plan != oldPlan,
	}
	
	if prorationCalc != nil {
		auditData["proration"] = map[string]interface{}{
			"credit":         prorationCalc.Credit,
			"charge_amount":  prorationCalc.ChargeAmount,
			"effective_price": prorationCalc.EffectivePrice,
		}
	}
	
	um.auditLogger.LogPlanEvent(audit.AuditEvent{
		EventType: audit.EventPlanChanged,
		UserID:    userID,
		Data:      auditData,
		Timestamp: user.UpdatedAt,
	})
	
	// Crear evento de webhook
	if prorationCalc != nil {
		webhookEvent := um.webhookManager.CreateUpgradeProratedEvent(userID, prorationCalc)
		um.webhookManager.QueueEvent(ctx, webhookEvent)
	} else {
		webhookEvent := um.webhookManager.CreatePlanChangedEvent(userID, oldPlan, newPlan, user.UpdatedAt)
		um.webhookManager.QueueEvent(ctx, webhookEvent)
	}
	
	um.logger.Info("User plan changed",
		"user_id", userID,
		"old_plan", oldPlan,
		"new_plan", newPlan,
		"has_proration", prorationCalc != nil,
	)
	
	return prorationCalc, nil
}

// UserExists verifica si un usuario existe
func (um *UserManager) UserExists(ctx context.Context, userID string) (bool, error) {
	exists, err := um.redis.Exists(ctx, um.keyUserData(userID)).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}
	return exists > 0, nil
}

// GetUserPlan obtiene el plan actual de un usuario
func (um *UserManager) GetUserPlan(ctx context.Context, userID string) (config.Plan, error) {
	plan, err := um.redis.Get(ctx, um.keyUserPlan(userID)).Result()
	if err != nil {
		if err == redis.Nil {
			return config.PlanFree, nil // Plan por defecto
		}
		return "", fmt.Errorf("failed to get user plan: %w", err)
	}
	
	return config.Plan(plan), nil
}

// GetUsersByPlan obtiene usuarios por plan
func (um *UserManager) GetUsersByPlan(ctx context.Context, plan config.Plan) ([]string, error) {
	users, err := um.redis.SMembers(ctx, um.keyUsersByPlan(plan)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get users by plan: %w", err)
	}
	return users, nil
}

// GetAllUsers obtiene todos los IDs de usuarios
func (um *UserManager) GetAllUsers(ctx context.Context) ([]string, error) {
	users, err := um.redis.SMembers(ctx, um.keyAllUsers()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get all users: %w", err)
	}
	return users, nil
}

// DeactivateUser desactiva un usuario
func (um *UserManager) DeactivateUser(ctx context.Context, userID string, reason string) error {
	user, err := um.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	
	oldStatus := user.Status
	user.Status = config.UserStatusInactive
	user.UpdatedAt = time.Now()
	
	if err := um.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user status: %w", err)
	}
	
	// Log de auditoría
	um.auditLogger.LogEvent(audit.AuditEvent{
		EventType: audit.EventPlanChanged,
		UserID:    userID,
		Data: map[string]interface{}{
			"action":     "user_deactivated",
			"old_status": oldStatus,
			"new_status": user.Status,
			"reason":     reason,
		},
		Timestamp: user.UpdatedAt,
	})
	
	// Crear evento de webhook
	webhookEvent := &WebhookEvent{
		Type:   WebhookUserDeactivated,
		UserID: userID,
		Data: map[string]interface{}{
			"user_id":       userID,
			"reason":        reason,
			"deactivated_at": user.UpdatedAt.Format(time.RFC3339),
		},
	}
	um.webhookManager.QueueEvent(ctx, webhookEvent)
	
	um.logger.Info("User deactivated",
		"user_id", userID,
		"reason", reason,
	)
	
	return nil
}

// Helper methods para generar keys de Redis
func (um *UserManager) keyUserData(userID string) string {
	return fmt.Sprintf("user:%s:data", userID)
}

func (um *UserManager) keyUserPlan(userID string) string {
	return fmt.Sprintf("user:%s:plan", userID)
}

func (um *UserManager) keyUsersByPlan(plan config.Plan) string {
	return fmt.Sprintf("users:plan:%s", plan)
}

func (um *UserManager) keyAllUsers() string {
	return "users:all"
}

func (um *UserManager) keyUserCreatedAt(userID string) string {
	return fmt.Sprintf("user:%s:created_at", userID)
}

// detectUserChanges detecta qué campos cambiaron en el usuario
func (um *UserManager) detectUserChanges(old, new *config.User) map[string]interface{} {
	changes := make(map[string]interface{})
	
	if old.Plan != new.Plan {
		changes["plan"] = map[string]interface{}{
			"old": old.Plan,
			"new": new.Plan,
		}
	}
	
	if old.Status != new.Status {
		changes["status"] = map[string]interface{}{
			"old": old.Status,
			"new": new.Status,
		}
	}
	
	if old.Email != new.Email {
		changes["email"] = map[string]interface{}{
			"old": old.Email,
			"new": new.Email,
		}
	}
	
	if old.Name != new.Name {
		changes["name"] = map[string]interface{}{
			"old": old.Name,
			"new": new.Name,
		}
	}
	
	if old.BillingCycle != new.BillingCycle {
		changes["billing_cycle"] = map[string]interface{}{
			"old": old.BillingCycle,
			"new": new.BillingCycle,
		}
	}
	
	return changes
}

// hasSignificantChanges verifica si hay cambios que ameriten un webhook
func (um *UserManager) hasSignificantChanges(changes map[string]interface{}) bool {
	significantFields := []string{"plan", "status", "billing_cycle"}
	
	for _, field := range significantFields {
		if _, exists := changes[field]; exists {
			return true
		}
	}
	
	return false
}