package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

const (
	// Tamaño del pool de procesos LibreOffice
	DefaultPoolSize = 3
	
	// Timeout para iniciar un proceso
	StartTimeout = 30 * time.Second
	
	// Timeout para operaciones de conversión
	ConversionTimeout = 5 * time.Minute
	
	// TTL de un proceso (después de este tiempo, se reinicia)
	ProcessTTL = 30 * time.Minute
	
	// Máximo de conversiones antes de reiniciar proceso
	MaxConversionsPerProcess = 100
)

var (
	ErrPoolClosed     = errors.New("pool is closed")
	ErrProcessUnhealthy = errors.New("process is unhealthy")
	ErrTimeout        = errors.New("operation timeout")
)

// LibreOfficeProcess representa un proceso de LibreOffice
type LibreOfficeProcess struct {
	cmd           *exec.Cmd
	port          int
	pid           int
	startedAt     time.Time
	conversions   int
	lastUsed      time.Time
	healthy       bool
	mu            sync.RWMutex
}

// LibreOfficePool pool de procesos LibreOffice warm
type LibreOfficePool struct {
	processes []*LibreOfficeProcess
	available chan *LibreOfficeProcess
	size      int
	logger    *logger.Logger
	closed    bool
	mu        sync.RWMutex
}

// NewLibreOfficePool crea un nuevo pool de procesos
func NewLibreOfficePool(size int, log *logger.Logger) (*LibreOfficePool, error) {
	if size <= 0 {
		size = DefaultPoolSize
	}
	
	pool := &LibreOfficePool{
		processes: make([]*LibreOfficeProcess, 0, size),
		available: make(chan *LibreOfficeProcess, size),
		size:      size,
		logger:    log,
		closed:    false,
	}
	
	// Inicializar procesos warm
	for i := 0; i < size; i++ {
		process, err := pool.startProcess(8100 + i)
		if err != nil {
			pool.logger.Error("Failed to start LibreOffice process",
				"port", 8100+i,
				"error", err,
			)
			continue
		}
		
		pool.processes = append(pool.processes, process)
		pool.available <- process
		
		pool.logger.Info("LibreOffice process started",
			"pid", process.pid,
			"port", process.port,
		)
	}
	
	if len(pool.processes) == 0 {
		return nil, errors.New("failed to start any LibreOffice processes")
	}
	
	// Iniciar goroutine de mantenimiento
	go pool.maintenance()
	
	return pool, nil
}

// startProcess inicia un nuevo proceso de LibreOffice
func (p *LibreOfficePool) startProcess(port int) (*LibreOfficeProcess, error) {
	ctx, cancel := context.WithTimeout(context.Background(), StartTimeout)
	defer cancel()
	
	// Comando para iniciar LibreOffice en modo headless
	cmd := exec.CommandContext(ctx,
		"soffice",
		"--headless",
		"--invisible",
		"--nocrashreport",
		"--nodefault",
		"--nofirststartwizard",
		"--nolockcheck",
		"--nologo",
		"--norestore",
		fmt.Sprintf("--accept=socket,host=127.0.0.1,port=%d;urp;", port),
	)
	
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}
	
	process := &LibreOfficeProcess{
		cmd:       cmd,
		port:      port,
		pid:       cmd.Process.Pid,
		startedAt: time.Now(),
		lastUsed:  time.Now(),
		healthy:   true,
	}
	
	// Esperar a que el proceso esté listo (verificar puerto)
	if err := p.waitForReady(process); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("process not ready: %w", err)
	}
	
	return process, nil
}

// waitForReady espera a que el proceso esté listo para aceptar conexiones
func (p *LibreOfficePool) waitForReady(process *LibreOfficeProcess) error {
	// Implementación simplificada - esperar 2 segundos
	// En producción, verificar que el puerto esté escuchando
	time.Sleep(2 * time.Second)
	return nil
}

// Acquire obtiene un proceso del pool
func (p *LibreOfficePool) Acquire(ctx context.Context) (*LibreOfficeProcess, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrPoolClosed
	}
	p.mu.RUnlock()
	
	select {
	case process := <-p.available:
		// Verificar salud del proceso
		process.mu.RLock()
		healthy := process.healthy
		process.mu.RUnlock()
		
		if !healthy {
			// Reiniciar proceso no saludable
			p.logger.Warn("Process unhealthy, restarting",
				"pid", process.pid,
				"port", process.port,
			)
			
			if err := p.restartProcess(process); err != nil {
				p.logger.Error("Failed to restart process", "error", err)
				return nil, ErrProcessUnhealthy
			}
		}
		
		// Actualizar last used
		process.mu.Lock()
		process.lastUsed = time.Now()
		process.mu.Unlock()
		
		return process, nil
		
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Release devuelve un proceso al pool
func (p *LibreOfficePool) Release(process *LibreOfficeProcess) {
	if process == nil {
		return
	}
	
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.closed {
		p.killProcess(process)
		return
	}
	
	// Incrementar contador de conversiones
	process.mu.Lock()
	process.conversions++
	conversions := process.conversions
	age := time.Since(process.startedAt)
	process.mu.Unlock()
	
	// Reiniciar si excede límites
	if conversions >= MaxConversionsPerProcess || age >= ProcessTTL {
		p.logger.Info("Process needs restart",
			"pid", process.pid,
			"conversions", conversions,
			"age", age,
		)
		
		if err := p.restartProcess(process); err != nil {
			p.logger.Error("Failed to restart process", "error", err)
			process.mu.Lock()
			process.healthy = false
			process.mu.Unlock()
		}
	}
	
	// Devolver al pool
	select {
	case p.available <- process:
		// Devuelto exitosamente
	default:
		// Pool lleno (no debería pasar)
		p.logger.Warn("Pool full, killing process", "pid", process.pid)
		p.killProcess(process)
	}
}

// Convert ejecuta una conversión usando un proceso del pool
func (p *LibreOfficePool) Convert(ctx context.Context, inputPath, outputPath string) error {
	// Adquirir proceso
	process, err := p.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire process: %w", err)
	}
	defer p.Release(process)
	
	// Ejecutar conversión con timeout
	convCtx, cancel := context.WithTimeout(ctx, ConversionTimeout)
	defer cancel()
	
	// Usar unoconv o pyodconverter con el puerto del proceso
	cmd := exec.CommandContext(convCtx,
		"unoconv",
		"-f", "pdf",
		"-o", outputPath,
		"--connection", fmt.Sprintf("socket,host=127.0.0.1,port=%d;urp;StarOffice.ComponentContext", process.port),
		inputPath,
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		p.logger.Error("Conversion failed",
			"pid", process.pid,
			"input", inputPath,
			"output", string(output),
			"error", err,
		)
		
		// Marcar proceso como no saludable si hay error
		process.mu.Lock()
		process.healthy = false
		process.mu.Unlock()
		
		return fmt.Errorf("conversion failed: %w", err)
	}
	
	p.logger.Debug("Conversion successful",
		"pid", process.pid,
		"input", inputPath,
		"output", outputPath,
	)
	
	return nil
}

// restartProcess reinicia un proceso
func (p *LibreOfficePool) restartProcess(process *LibreOfficeProcess) error {
	// Matar proceso viejo
	p.killProcess(process)
	
	// Iniciar nuevo proceso
	newProcess, err := p.startProcess(process.port)
	if err != nil {
		return err
	}
	
	// Actualizar referencia
	process.mu.Lock()
	process.cmd = newProcess.cmd
	process.pid = newProcess.pid
	process.startedAt = newProcess.startedAt
	process.conversions = 0
	process.healthy = true
	process.mu.Unlock()
	
	p.logger.Info("Process restarted",
		"pid", newProcess.pid,
		"port", process.port,
	)
	
	return nil
}

// killProcess mata un proceso
func (p *LibreOfficePool) killProcess(process *LibreOfficeProcess) {
	if process == nil || process.cmd == nil || process.cmd.Process == nil {
		return
	}
	
	if err := process.cmd.Process.Kill(); err != nil {
		p.logger.Error("Failed to kill process",
			"pid", process.pid,
			"error", err,
		)
	}
}

// maintenance goroutine de mantenimiento del pool
func (p *LibreOfficePool) maintenance() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		p.mu.RLock()
		if p.closed {
			p.mu.RUnlock()
			return
		}
		p.mu.RUnlock()
		
		// Verificar salud de procesos
		for _, process := range p.processes {
			process.mu.RLock()
			idle := time.Since(process.lastUsed)
			conversions := process.conversions
			age := time.Since(process.startedAt)
			process.mu.RUnlock()
			
			// Reiniciar si está idle por mucho tiempo o es muy viejo
			if idle > 15*time.Minute || age > ProcessTTL || conversions > MaxConversionsPerProcess {
				p.logger.Info("Maintenance: restarting process",
					"pid", process.pid,
					"idle", idle,
					"age", age,
					"conversions", conversions,
				)
				
				if err := p.restartProcess(process); err != nil {
					p.logger.Error("Maintenance: failed to restart", "error", err)
				}
			}
		}
	}
}

// Close cierra el pool y mata todos los procesos
func (p *LibreOfficePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.closed {
		return nil
	}
	
	p.closed = true
	close(p.available)
	
	// Matar todos los procesos
	for _, process := range p.processes {
		p.killProcess(process)
	}
	
	p.logger.Info("LibreOffice pool closed", "size", len(p.processes))
	return nil
}

// Stats retorna estadísticas del pool
func (p *LibreOfficePool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	stats := map[string]interface{}{
		"size":      p.size,
		"available": len(p.available),
		"processes": len(p.processes),
		"closed":    p.closed,
	}
	
	var totalConversions int
	var healthyCount int
	
	for _, process := range p.processes {
		process.mu.RLock()
		totalConversions += process.conversions
		if process.healthy {
			healthyCount++
		}
		process.mu.RUnlock()
	}
	
	stats["total_conversions"] = totalConversions
	stats["healthy_count"] = healthyCount
	
	return stats
}
