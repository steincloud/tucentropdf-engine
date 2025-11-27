package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// JobStatusStore almacena y recupera estado de jobs en Redis
type JobStatusStore struct {
	redis  *redis.Client
	logger *logger.Logger
	ttl    time.Duration // Time to live para jobs completados
}

// NewJobStatusStore crea un nuevo store de status
func NewJobStatusStore(redisClient *redis.Client, log *logger.Logger) *JobStatusStore {
	return &JobStatusStore{
		redis:  redisClient,
		logger: log,
		ttl:    24 * time.Hour, // Retener jobs completados por 24 horas
	}
}

// SaveJobStatus guarda el estado de un job
func (s *JobStatusStore) SaveJobStatus(ctx context.Context, result *JobResult) error {
	key := fmt.Sprintf("job:status:%s", result.JobID)
	
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal job result: %w", err)
	}

	// Guardar con TTL
	err = s.redis.Set(ctx, key, data, s.ttl).Err()
	if err != nil {
		s.logger.Error("Failed to save job status",
			"job_id", result.JobID,
			"error", err,
		)
		return fmt.Errorf("failed to save job status: %w", err)
	}

	// Actualizar índice de jobs por usuario
	userJobsKey := fmt.Sprintf("user:jobs:%s", result.Metadata["user_id"])
	s.redis.SAdd(ctx, userJobsKey, result.JobID)
	s.redis.Expire(ctx, userJobsKey, 7*24*time.Hour) // 7 días

	s.logger.Debug("Job status saved",
		"job_id", result.JobID,
		"status", result.Status,
	)

	return nil
}

// GetJobStatus obtiene el estado de un job
func (s *JobStatusStore) GetJobStatus(ctx context.Context, jobID string) (*JobResult, error) {
	key := fmt.Sprintf("job:status:%s", jobID)
	
	data, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job status: %w", err)
	}

	var result JobResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job result: %w", err)
	}

	return &result, nil
}

// GetUserJobs obtiene todos los jobs de un usuario
func (s *JobStatusStore) GetUserJobs(ctx context.Context, userID string, limit int) ([]*JobResult, error) {
	userJobsKey := fmt.Sprintf("user:jobs:%s", userID)
	
	// Obtener IDs de jobs
	jobIDs, err := s.redis.SMembers(ctx, userJobsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user jobs: %w", err)
	}

	if len(jobIDs) == 0 {
		return []*JobResult{}, nil
	}

	// Aplicar límite
	if limit > 0 && len(jobIDs) > limit {
		jobIDs = jobIDs[:limit]
	}

	// Obtener detalles de cada job
	results := make([]*JobResult, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		result, err := s.GetJobStatus(ctx, jobID)
		if err != nil {
			s.logger.Warn("Failed to get job details", "job_id", jobID, "error", err)
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// UpdateJobProgress actualiza el progreso de un job
func (s *JobStatusStore) UpdateJobProgress(ctx context.Context, jobID string, progress int, message string) error {
	result, err := s.GetJobStatus(ctx, jobID)
	if err != nil {
		// Si el job no existe aún, crear uno nuevo con estado processing
		result = &JobResult{
			JobID:    jobID,
			Status:   StatusProcessing,
			Metadata: make(map[string]string),
		}
	}

	// Actualizar progreso
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["progress"] = fmt.Sprintf("%d", progress)
	result.Metadata["progress_message"] = message

	return s.SaveJobStatus(ctx, result)
}

// DeleteJob elimina un job del store
func (s *JobStatusStore) DeleteJob(ctx context.Context, jobID string) error {
	key := fmt.Sprintf("job:status:%s", jobID)
	
	err := s.redis.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	s.logger.Info("Job deleted", "job_id", jobID)
	return nil
}

// GetPendingJobsCount obtiene el número de jobs pendientes
func (s *JobStatusStore) GetPendingJobsCount(ctx context.Context) (int64, error) {
	// Escanear todas las keys de jobs
	var cursor uint64
	var count int64

	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "job:status:*", 100).Result()
		if err != nil {
			return 0, err
		}

		// Verificar estado de cada job
		for _, key := range keys {
			data, err := s.redis.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			var result JobResult
			if err := json.Unmarshal([]byte(data), &result); err != nil {
				continue
			}

			if result.Status == StatusPending || result.Status == StatusProcessing {
				count++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// CleanupExpiredJobs limpia jobs expirados (más de 24h completados)
func (s *JobStatusStore) CleanupExpiredJobs(ctx context.Context) (int, error) {
	var cursor uint64
	var deleted int

	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "job:status:*", 100).Result()
		if err != nil {
			return deleted, err
		}

		// Verificar expiración de cada job
		for _, key := range keys {
			// Redis ya maneja TTL automáticamente, pero podemos hacer limpieza adicional
			ttl := s.redis.TTL(ctx, key).Val()
			if ttl < 0 {
				// Key sin TTL o expirada
				s.redis.Del(ctx, key)
				deleted++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if deleted > 0 {
		s.logger.Info("Cleaned up expired jobs", "count", deleted)
	}

	return deleted, nil
}
