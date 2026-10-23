package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/cv-projects/uber-matching/internal/matching"
	"github.com/cv-projects/uber-matching/internal/storage"
	"github.com/redis/go-redis/v9"
)

type MockFaultyDB struct {
	mu           sync.Mutex
	isCrashed    bool
	storedEvents map[string]storage.TripEvent
	callCount    int
}

func NewMockFaultyDB() *MockFaultyDB {
	return &MockFaultyDB{
		storedEvents: make(map[string]storage.TripEvent),
	}
}

func (m *MockFaultyDB) SetSimulateCrash(crashed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isCrashed = crashed
}

func (m *MockFaultyDB) SaveEventsBatch(ctx context.Context, events []storage.TripEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	if m.isCrashed {
		return errors.New("connection refused: postgresql database container is DOWN")
	}

	for _, e := range events {
		// Idempotent insertion by event_id (mimicking ON CONFLICT DO NOTHING)
		m.storedEvents[e.EventID] = e
	}
	return nil
}

func (m *MockFaultyDB) TotalStored() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.storedEvents)
}

func (m *MockFaultyDB) Close() {}

func TestChaos_DatabaseCrashAndRecovery(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	mockDB := NewMockFaultyDB()
	engine := matching.NewMatchingEngine(client)
	worker := NewArchiverWorker(client, mockDB)

	ctx := context.Background()
	if err := worker.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("failed to init consumer group: %v", err)
	}

	// 1. SIMULAMOS CAÍDA ABRUPTA DE POSTGRESQL (Crash / Network Cut)
	t.Log("[CHAOS] Simulando caída abrupta de PostgreSQL...")
	mockDB.SetSimulateCrash(true)

	// 2. Iniciamos el worker en background mientras la base de datos está caída
	worker.Start()

	// 3. Emitimos un pico de 2.000 asignaciones concurrentes en el motor
	const numEvents = 2000
	t.Logf("[CHAOS] Inyectando %d asignaciones en Redis mientras la DB está caída...", numEvents)

	for i := 0; i < numEvents; i++ {
		tripID := fmt.Sprintf("trip-chaos-%d", i)
		driverID := fmt.Sprintf("driver-chaos-%d", i)

		_ = engine.CreateTrip(ctx, tripID, "rider-1")
		_ = engine.SetDriverStatus(ctx, driverID, matching.DriverStatusAvailable)

		ok, err := engine.AssignDriver(ctx, tripID, driverID)
		if err != nil || !ok {
			t.Fatalf("AssignDriver failed at %d: %v", i, err)
		}
	}

	// Esperamos unos ciclos para certificar que el worker falla al escribir en la DB
	time.Sleep(300 * time.Millisecond)

	// Verificamos que Postgres NO ha recibido datos (está caída)
	if stored := mockDB.TotalStored(); stored != 0 {
		t.Fatalf("Anomalía: la base de datos caída no debería tener registros, tiene %d", stored)
	}

	t.Logf("[CHAOS] Worker intentó escribir, falló y retuvo los eventos en el Stream sin ejecutar XACK. Intentos fallidos: %d", worker.FailedBatches())

	// 4. RECUPERAMOS LA BASE DE DATOS (Container reboot / Network healed)
	t.Log("[CHAOS] Recuperando conexión a PostgreSQL...")
	mockDB.SetSimulateCrash(false)

	// Damos tiempo al worker para drenar los 2.000 eventos de la Pending Entries List (PEL)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if mockDB.TotalStored() == numEvents {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 5. VALIDACIÓN DE CONSISTENCIA EVENTUAL
	totalCommitted := mockDB.TotalStored()
	t.Logf("[CHAOS] Total de eventos consolidados en PostgreSQL tras recuperación: %d / %d", totalCommitted, numEvents)

	if totalCommitted != numEvents {
		t.Fatalf("PÉRDIDA DE DATOS DETECTADA: Se esperaban %d registros en DB, solo hay %d", numEvents, totalCommitted)
	}

	// 6. APAGADO ORDENADO (Graceful Shutdown)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	worker.Stop(shutdownCtx)
}

func BenchmarkHotPath_AssignDriverWithXAdd(b *testing.B) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	engine := matching.NewMatchingEngine(client)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tripID := fmt.Sprintf("bench-xadd-trip-%d", i)
		driverID := fmt.Sprintf("bench-xadd-driver-%d", i)

		_ = engine.CreateTrip(ctx, tripID, "rider-bench")
		_ = engine.SetDriverStatus(ctx, driverID, matching.DriverStatusAvailable)

		_, _ = engine.AssignDriver(ctx, tripID, driverID)
	}
}
