package config

import (
	"fmt"
	"math"
	"time"
)

// ProrationService servicio para calcular prorrateos entre planes
type ProrationService struct {
	planConfig *PlanConfiguration
}

// NewProrationService crea un nuevo servicio de prorrateo
func NewProrationService() *ProrationService {
	return &ProrationService{
		planConfig: GetDefaultPlanConfiguration(),
	}
}

// ProrationCalculation resultado del cálculo de prorrateo
type ProrationCalculation struct {
	// Plan actual
	CurrentPlan Plan    `json:"current_plan"`
	CurrentPrice float64 `json:"current_price"`
	
	// Nuevo plan
	NewPlan  Plan    `json:"new_plan"`
	NewPrice float64 `json:"new_price"`
	
	// Información del ciclo
	BillingCycle   BillingCycle `json:"billing_cycle"`
	CycleDuration  time.Duration `json:"cycle_duration"`
	TimeUsed       time.Duration `json:"time_used"`
	TimeRemaining  time.Duration `json:"time_remaining"`
	
	// Cálculos de prorrateo
	UnusedValue    float64 `json:"unused_value"`     // Valor no usado del plan actual
	Credit         float64 `json:"credit"`           // Crédito a aplicar
	ChargeAmount   float64 `json:"charge_amount"`    // Monto adicional a cobrar
	EffectivePrice float64 `json:"effective_price"`  // Precio efectivo después del crédito
	
	// Información adicional
	IsUpgrade      bool   `json:"is_upgrade"`
	SavingsPercent float64 `json:"savings_percent,omitempty"` // Porcentaje de ahorro en upgrade
	
	// Para integración con Lemon Squeezy
	LemonSqueezyData *LemonSqueezyProrationData `json:"lemon_squeezy_data,omitempty"`
}

// LemonSqueezyProrationData datos específicos para Lemon Squeezy
type LemonSqueezyProrationData struct {
	SubscriptionID   string  `json:"subscription_id"`
	CurrentVariantID string  `json:"current_variant_id"`
	NewVariantID     string  `json:"new_variant_id"`
	ProrationCredit  float64 `json:"proration_credit"`  // En centavos
	UpgradeAmount    float64 `json:"upgrade_amount"`    // En centavos
	EffectiveDate    string  `json:"effective_date"`    // ISO 8601
}

// CalcularProrrateo calcula el prorrateo para cambio de plan
func (ps *ProrationService) CalcularProrrateo(
	planActual Plan,
	nuevoPlan Plan,
	ciclo BillingCycle,
	tiempoUsado time.Duration,
	duracionCiclo time.Duration,
) (*ProrationCalculation, error) {
	// Validar parámetros
	if err := ps.validateProrationParams(planActual, nuevoPlan, ciclo, tiempoUsado, duracionCiclo); err != nil {
		return nil, err
	}
	
	// Obtener precios de los planes
	currentPricing := ps.planConfig.GetPlanPricing(planActual)
	newPricing := ps.planConfig.GetPlanPricing(nuevoPlan)
	
	// Obtener precio según ciclo
	var currentPrice, newPrice float64
	if ciclo == BillingMonthly {
		currentPrice = currentPricing.Monthly
		newPrice = newPricing.Monthly
	} else {
		currentPrice = currentPricing.Yearly
		newPrice = newPricing.Yearly
	}
	
	// Calcular tiempo restante
	tiempoRestante := duracionCiclo - tiempoUsado
	if tiempoRestante < 0 {
		tiempoRestante = 0
	}
	
	// Calcular valor no usado del plan actual
	unusedRatio := float64(tiempoRestante) / float64(duracionCiclo)
	unusedValue := currentPrice * unusedRatio
	
	// El crédito es el valor no usado del plan actual
	credit := unusedValue
	
	// Calcular monto a cobrar (precio nuevo menos crédito)
	chargeAmount := newPrice - credit
	
	// Asegurar que nunca sea negativo
	if chargeAmount < 0 {
		chargeAmount = 0
		credit = newPrice // Ajustar crédito para que no exceda el precio nuevo
	}
	
	// Calcular precio efectivo
	effectivePrice := chargeAmount
	
	// Verificar si es upgrade
	isUpgrade := ps.isPlanUpgrade(planActual, nuevoPlan)
	
	// Calcular porcentaje de ahorro si aplica
	var savingsPercent float64
	if isUpgrade && credit > 0 {
		savingsPercent = (credit / newPrice) * 100
	}
	
	calculation := &ProrationCalculation{
		CurrentPlan:    planActual,
		CurrentPrice:   currentPrice,
		NewPlan:        nuevoPlan,
		NewPrice:       newPrice,
		BillingCycle:   ciclo,
		CycleDuration:  duracionCiclo,
		TimeUsed:       tiempoUsado,
		TimeRemaining:  tiempoRestante,
		UnusedValue:    unusedValue,
		Credit:         credit,
		ChargeAmount:   chargeAmount,
		EffectivePrice: effectivePrice,
		IsUpgrade:      isUpgrade,
		SavingsPercent: math.Round(savingsPercent*100)/100, // Redondear a 2 decimales
	}
	
	// Agregar datos para Lemon Squeezy
	calculation.LemonSqueezyData = ps.generateLemonSqueezyData(calculation)
	
	return calculation, nil
}

// CalcularProrrateoSimple versión simplificada para casos comunes
func (ps *ProrationService) CalcularProrrateoSimple(
	planActual Plan,
	nuevoPlan Plan,
	diasUsados int,
	cicloDias int,
) (*ProrationCalculation, error) {
	
	tiempoUsado := time.Duration(diasUsados) * 24 * time.Hour
	duracionCiclo := time.Duration(cicloDias) * 24 * time.Hour
	
	// Determinar ciclo basado en duración
	var ciclo BillingCycle
	if cicloDias <= 31 {
		ciclo = BillingMonthly
	} else {
		ciclo = BillingYearly
	}
	
	return ps.CalcularProrrateo(planActual, nuevoPlan, ciclo, tiempoUsado, duracionCiclo)
}

// CalcularProrrateoMensual cálculo específico para ciclo mensual
func (ps *ProrationService) CalcularProrrateoMensual(
	planActual Plan,
	nuevoPlan Plan,
	diaActual int, // Día del mes actual (1-31)
) (*ProrationCalculation, error) {
	
	now := time.Now()
	
	// Calcular días usados y duración del ciclo
	diasUsados := diaActual - 1 // Días transcurridos (0-based)
	if diasUsados < 0 {
		diasUsados = 0
	}
	
	// Duración del mes actual
	diasEnMes := getDaysInMonth(now.Year(), now.Month())
	
	return ps.CalcularProrrateoSimple(planActual, nuevoPlan, diasUsados, diasEnMes)
}

// CalcularProrrateoAnual cálculo específico para ciclo anual
func (ps *ProrationService) CalcularProrrateoAnual(
	planActual Plan,
	nuevoPlan Plan,
	fechaInicio time.Time,
) (*ProrationCalculation, error) {
	
	now := time.Now()
	
	// Calcular tiempo transcurrido desde el inicio
	tiempoUsado := now.Sub(fechaInicio)
	if tiempoUsado < 0 {
		tiempoUsado = 0
	}
	
	// Duración del año
	var duracionCiclo time.Duration
	if isLeapYear(fechaInicio.Year()) {
		duracionCiclo = 366 * 24 * time.Hour
	} else {
		duracionCiclo = 365 * 24 * time.Hour
	}
	
	return ps.CalcularProrrateo(planActual, nuevoPlan, BillingYearly, tiempoUsado, duracionCiclo)
}

// validateProrationParams valida los parámetros de entrada
func (ps *ProrationService) validateProrationParams(
	planActual Plan,
	nuevoPlan Plan,
	ciclo BillingCycle,
	tiempoUsado time.Duration,
	duracionCiclo time.Duration,
) error {
	
	if !planActual.IsValid() {
		return fmt.Errorf("plan actual inválido: %s", planActual)
	}
	
	if !nuevoPlan.IsValid() {
		return fmt.Errorf("nuevo plan inválido: %s", nuevoPlan)
	}
	
	if planActual == nuevoPlan {
		return fmt.Errorf("el plan actual y nuevo son iguales")
	}
	
	if ciclo != BillingMonthly && ciclo != BillingYearly {
		return fmt.Errorf("ciclo de facturación inválido: %s", ciclo)
	}
	
	if tiempoUsado < 0 {
		return fmt.Errorf("tiempo usado no puede ser negativo")
	}
	
	if duracionCiclo <= 0 {
		return fmt.Errorf("duración del ciclo debe ser positiva")
	}
	
	if tiempoUsado > duracionCiclo {
		return fmt.Errorf("tiempo usado no puede exceder la duración del ciclo")
	}
	
	return nil
}

// isPlanUpgrade determina si el cambio es un upgrade
func (ps *ProrationService) isPlanUpgrade(currentPlan, newPlan Plan) bool {
	planHierarchy := map[Plan]int{
		PlanFree:    1,
		PlanPremium: 2,
		PlanPro:     3,
	}
	
	currentLevel := planHierarchy[currentPlan]
	newLevel := planHierarchy[newPlan]
	
	return newLevel > currentLevel
}

// generateLemonSqueezyData genera datos específicos para Lemon Squeezy
func (ps *ProrationService) generateLemonSqueezyData(calc *ProrationCalculation) *LemonSqueezyProrationData {
	// IDs de variantes de Lemon Squeezy (estos deberían venir de configuración)
	variantIDs := map[string]string{
		string(PlanFree) + "_monthly":    "",        // Free no tiene variant ID
		string(PlanPremium) + "_monthly": "premium_monthly_variant",
		string(PlanPremium) + "_yearly":  "premium_yearly_variant",
		string(PlanPro) + "_monthly":     "pro_monthly_variant",
		string(PlanPro) + "_yearly":      "pro_yearly_variant",
	}
	
	currentKey := string(calc.CurrentPlan) + "_" + string(calc.BillingCycle)
	newKey := string(calc.NewPlan) + "_" + string(calc.BillingCycle)
	
	// Convertir a centavos para Lemon Squeezy
	prorationCreditCents := calc.Credit * 100
	upgradeAmountCents := calc.ChargeAmount * 100
	
	return &LemonSqueezyProrationData{
		SubscriptionID:   "",                                    // Se debe proporcionar externamente
		CurrentVariantID: variantIDs[currentKey],
		NewVariantID:     variantIDs[newKey],
		ProrationCredit:  math.Round(prorationCreditCents),
		UpgradeAmount:    math.Round(upgradeAmountCents),
		EffectiveDate:    time.Now().Format("2006-01-02T15:04:05Z"), // ISO 8601
	}
}

// GetProrationEstimate obtiene una estimación rápida de prorrateo
func (ps *ProrationService) GetProrationEstimate(currentPlan, newPlan Plan, daysUsed, totalDays int) (float64, float64, error) {
	calc, err := ps.CalcularProrrateoSimple(currentPlan, newPlan, daysUsed, totalDays)
	if err != nil {
		return 0, 0, err
	}
	
	return calc.Credit, calc.ChargeAmount, nil
}

// Utility functions

// getDaysInMonth retorna el número de días en un mes específico
func getDaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// isLeapYear verifica si un año es bisiesto
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// FormatDuration formatea una duración de manera amigable
func FormatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	
	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%d días y %d horas", days, hours)
		}
		return fmt.Sprintf("%d días", days)
	}
	
	if hours > 0 {
		return fmt.Sprintf("%d horas", hours)
	}
	
	minutes := int(d.Minutes())
	return fmt.Sprintf("%d minutos", minutes)
}