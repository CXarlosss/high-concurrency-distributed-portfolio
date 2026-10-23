package matching

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return client, cleanup
}

func TestEnjambre_MultipleDriversOneTrip(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	engine := NewMatchingEngine(client)
	ctx := context.Background()

	tripID := "trip-high-demand-100"
	if err := engine.CreateTrip(ctx, tripID, "rider-1"); err != nil {
		t.Fatalf("failed to create trip: %v", err)
	}

	const numDrivers = 100
	for i := 0; i < numDrivers; i++ {
		driverID := fmt.Sprintf("driver-%d", i)
		if err := engine.SetDriverStatus(ctx, driverID, DriverStatusAvailable); err != nil {
			t.Fatalf("failed to set driver status: %v", err)
		}
	}

	var successCount int64
	var failCount int64
	var winningDriver string
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(numDrivers)
	startLatch := make(chan struct{})

	// Launch 100 concurrent drivers firing simultaneously
	for i := 0; i < numDrivers; i++ {
		go func(id int) {
			defer wg.Done()
			driverID := fmt.Sprintf("driver-%d", id)

			<-startLatch // Synchronized explosive dispatch

			ok, err := engine.AssignDriver(ctx, tripID, driverID)
			if err != nil {
				t.Errorf("AssignDriver returned error for driver %s: %v", driverID, err)
				return
			}

			if ok {
				atomic.AddInt64(&successCount, 1)
				mu.Lock()
				winningDriver = driverID
				mu.Unlock()
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}(i)
	}

	// FIRE!
	close(startLatch)
	wg.Wait()

	t.Logf("Resultados Enjambre: Exitosos = %d, Fallidos = %d, Conductor Ganador = %s", successCount, failCount, winningDriver)

	if successCount != 1 {
		t.Fatalf("VIOLACIÓN DE CONSISTENCIA: Se esperaban exactamente 1 asignación exitosa, se obtuvieron %d", successCount)
	}
	if failCount != numDrivers-1 {
		t.Fatalf("Se esperaban %d rechazos, se obtuvieron %d", numDrivers-1, failCount)
	}

	// Verify Trip state in Redis
	state, assignedDriver, err := engine.GetTripState(ctx, tripID)
	if err != nil {
		t.Fatalf("failed to get trip state: %v", err)
	}
	if state != TripStateMatched || assignedDriver != winningDriver {
		t.Fatalf("Trip state corrupt: state=%s, driver=%s (expected MATCHED with %s)", state, assignedDriver, winningDriver)
	}

	// Verify winning driver status is BUSY
	status, err := engine.GetDriverStatus(ctx, winningDriver)
	if err != nil {
		t.Fatalf("failed to get driver status: %v", err)
	}
	if status != DriverStatusBusy {
		t.Fatalf("expected winning driver to be BUSY, got %s", status)
	}
}

func TestConductorOmnipresente_OneDriverMultipleTrips(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	engine := NewMatchingEngine(client)
	ctx := context.Background()

	driverID := "driver-busy-target"
	if err := engine.SetDriverStatus(ctx, driverID, DriverStatusAvailable); err != nil {
		t.Fatalf("failed to set driver status: %v", err)
	}

	const numTrips = 50
	for i := 0; i < numTrips; i++ {
		tripID := fmt.Sprintf("trip-request-%d", i)
		if err := engine.CreateTrip(ctx, tripID, fmt.Sprintf("rider-%d", i)); err != nil {
			t.Fatalf("failed to create trip %s: %v", tripID, err)
		}
	}

	var successCount int64
	var failCount int64
	var wonTrip string
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(numTrips)
	startLatch := make(chan struct{})

	// 50 concurrent trips trying to claim the same driver
	for i := 0; i < numTrips; i++ {
		go func(id int) {
			defer wg.Done()
			tripID := fmt.Sprintf("trip-request-%d", id)

			<-startLatch

			ok, err := engine.AssignDriver(ctx, tripID, driverID)
			if err != nil {
				t.Errorf("error in assign: %v", err)
				return
			}

			if ok {
				atomic.AddInt64(&successCount, 1)
				mu.Lock()
				wonTrip = tripID
				mu.Unlock()
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}(i)
	}

	close(startLatch)
	wg.Wait()

	t.Logf("Resultados Conductor Omnipresente: Exitosos = %d, Fallidos = %d, Viaje Ganador = %s", successCount, failCount, wonTrip)

	if successCount != 1 {
		t.Fatalf("DOBLE ASIGNACIÓN DETECTADA: El conductor fue asignado a %d viajes simultáneamente", successCount)
	}
	if failCount != numTrips-1 {
		t.Fatalf("Se esperaban %d rechazos, se obtuvieron %d", numTrips-1, failCount)
	}

	// Verify other 49 trips are still PENDING
	for i := 0; i < numTrips; i++ {
		tripID := fmt.Sprintf("trip-request-%d", i)
		state, _, err := engine.GetTripState(ctx, tripID)
		if err != nil {
			t.Fatalf("failed to check trip: %v", err)
		}

		if tripID == wonTrip {
			if state != TripStateMatched {
				t.Fatalf("Winning trip %s should be MATCHED, got %s", tripID, state)
			}
		} else {
			if state != TripStatePending {
				t.Fatalf("Ghost assignment! Trip %s should still be PENDING, got %s", tripID, state)
			}
		}
	}
}

func BenchmarkAssignDriver(b *testing.B) {
	client, cleanup := setupTestRedis(nil)
	defer cleanup()

	engine := NewMatchingEngine(client)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tripID := fmt.Sprintf("bench-trip-%d", i)
		driverID := fmt.Sprintf("bench-driver-%d", i)

		_ = engine.CreateTrip(ctx, tripID, "rider-bench")
		_ = engine.SetDriverStatus(ctx, driverID, DriverStatusAvailable)

		ok, err := engine.AssignDriver(ctx, tripID, driverID)
		if err != nil || !ok {
			b.Fatalf("AssignDriver failed at iteration %d: %v", i, err)
		}
	}
}
