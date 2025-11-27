package storage

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

func TestFileTracker_MarkInUse(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	filePath := "/tmp/test.pdf"

	// Marcar como en uso
	tracker.MarkInUse(filePath)

	if !tracker.IsInUse(filePath) {
		t.Error("File should be marked as in use")
	}

	if tracker.GetRefCount(filePath) != 1 {
		t.Errorf("Expected ref count 1, got %d", tracker.GetRefCount(filePath))
	}
}

func TestFileTracker_MultipleReferences(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	filePath := "/tmp/test.pdf"

	// Marcar 3 veces
	tracker.MarkInUse(filePath)
	tracker.MarkInUse(filePath)
	tracker.MarkInUse(filePath)

	if tracker.GetRefCount(filePath) != 3 {
		t.Errorf("Expected ref count 3, got %d", tracker.GetRefCount(filePath))
	}

	// Liberar 2 veces
	tracker.MarkAvailable(filePath)
	tracker.MarkAvailable(filePath)

	if tracker.GetRefCount(filePath) != 1 {
		t.Errorf("Expected ref count 1 after 2 releases, got %d", tracker.GetRefCount(filePath))
	}

	// Liberar última referencia
	tracker.MarkAvailable(filePath)

	if tracker.IsInUse(filePath) {
		t.Error("File should not be in use after all references released")
	}
}

func TestFileTracker_ConcurrentAccess(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	filePath := "/tmp/concurrent.pdf"
	var wg sync.WaitGroup

	// 100 goroutines incrementando
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.MarkInUse(filePath)
		}()
	}

	wg.Wait()

	expectedCount := int32(100)
	actualCount := tracker.GetRefCount(filePath)

	if actualCount != expectedCount {
		t.Errorf("Expected ref count %d, got %d", expectedCount, actualCount)
	}

	// 100 goroutines decrementando
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.MarkAvailable(filePath)
		}()
	}

	wg.Wait()

	if tracker.IsInUse(filePath) {
		t.Error("File should not be in use after all concurrent releases")
	}
}

func TestAtomicCleanup_BasicCleanup(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	// Crear directorio temporal para test
	tempDir := t.TempDir()

	cleanup := NewAtomicCleanup(tracker, log, tempDir, 100*time.Millisecond)

	// Crear archivo antiguo
	oldFile := filepath.Join(tempDir, "old.txt")
	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Esperar a que sea "antiguo"
	time.Sleep(150 * time.Millisecond)

	// Ejecutar cleanup
	ctx := context.Background()
	deleted, err := cleanup.CleanupOldFiles(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 file deleted, got %d", deleted)
	}

	// Verificar que el archivo fue eliminado
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("Old file should have been deleted")
	}
}

func TestAtomicCleanup_SkipInUseFiles(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	tempDir := t.TempDir()
	cleanup := NewAtomicCleanup(tracker, log, tempDir, 100*time.Millisecond)

	// Crear archivo y marcarlo como en uso
	inUseFile := filepath.Join(tempDir, "inuse.txt")
	if err := os.WriteFile(inUseFile, []byte("in use"), 0644); err != nil {
		t.Fatal(err)
	}

	tracker.MarkInUse(inUseFile)

	// Esperar a que sea "antiguo"
	time.Sleep(150 * time.Millisecond)

	// Ejecutar cleanup
	ctx := context.Background()
	deleted, err := cleanup.CleanupOldFiles(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 files deleted (file in use), got %d", deleted)
	}

	// Verificar que el archivo NO fue eliminado
	if _, err := os.Stat(inUseFile); os.IsNotExist(err) {
		t.Error("In-use file should NOT have been deleted")
	}

	// Liberar y ejecutar cleanup nuevamente
	tracker.MarkAvailable(inUseFile)
	deleted, err = cleanup.CleanupOldFiles(ctx)
	if err != nil {
		t.Fatalf("Second cleanup failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 file deleted after release, got %d", deleted)
	}
}

func TestAtomicCleanup_PreventConcurrentCleanup(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	tempDir := t.TempDir()
	cleanup := NewAtomicCleanup(tracker, log, tempDir, 50*time.Millisecond)

	var wg sync.WaitGroup
	errors := make(chan error, 3)

	// Intentar 3 cleanups concurrentes
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, err := cleanup.CleanupOldFiles(ctx)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Al menos 2 deberían fallar por cleanup ya en progreso
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount < 2 {
		t.Errorf("Expected at least 2 concurrent cleanup attempts to fail, got %d", errorCount)
	}
}

func TestAtomicCleanup_ContextCancellation(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	tempDir := t.TempDir()
	cleanup := NewAtomicCleanup(tracker, log, tempDir, 50*time.Millisecond)

	// Crear muchos archivos
	for i := 0; i < 100; i++ {
		file := filepath.Join(tempDir, filepath.Base(t.Name())+"-file-"+string(rune(i))+".txt")
		os.WriteFile(file, []byte("content"), 0644)
	}

	time.Sleep(100 * time.Millisecond)

	// Crear context con timeout muy corto
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Ejecutar cleanup
	_, err := cleanup.CleanupOldFiles(ctx)

	// Debería cancelarse por timeout
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

func TestAtomicCleanup_GetStats(t *testing.T) {
	log := logger.New("info", "json")
	tracker := NewFileTracker(log)

	tempDir := t.TempDir()
	maxAge := 1 * time.Hour
	cleanup := NewAtomicCleanup(tracker, log, tempDir, maxAge)

	// Marcar algunos archivos como en uso
	tracker.MarkInUse("/tmp/file1.txt")
	tracker.MarkInUse("/tmp/file2.txt")
	tracker.MarkInUse("/tmp/file3.txt")

	stats := cleanup.GetStats()

	if stats["files_in_use"].(int) != 3 {
		t.Errorf("Expected 3 files in use, got %d", stats["files_in_use"])
	}

	if stats["temp_dir"].(string) != tempDir {
		t.Errorf("Expected temp_dir %s, got %s", tempDir, stats["temp_dir"])
	}

	if stats["max_age"].(string) != maxAge.String() {
		t.Errorf("Expected max_age %s, got %s", maxAge, stats["max_age"])
	}

	t.Logf("Cleanup stats: %+v", stats)
}

func BenchmarkFileTracker_MarkInUse(b *testing.B) {
	log := logger.New("error", "json")
	tracker := NewFileTracker(log)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.MarkInUse("/tmp/bench.pdf")
	}
}

func BenchmarkFileTracker_Concurrent(b *testing.B) {
	log := logger.New("error", "json")
	tracker := NewFileTracker(log)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tracker.MarkInUse("/tmp/concurrent.pdf")
			tracker.MarkAvailable("/tmp/concurrent.pdf")
		}
	})
}
