package matching

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestAtomicCancellation_PreventsZombieDriver(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	engine := NewMatchingEngine(client)
	ctx := context.Background()

	tripID := "trip-cancel-test-1"
	driverID := "driver-cancel-test-1"

	_ = engine.CreateTrip(ctx, tripID, "rider-1")
	_ = engine.SetDriverStatus(ctx, driverID, DriverStatusAvailable)

	// 1. Assign driver
	ok, err := engine.AssignDriver(ctx, tripID, driverID)
	if err != nil || !ok {
		t.Fatalf("failed to assign: %v", err)
	}

	// Driver is BUSY
	status, _ := engine.GetDriverStatus(ctx, driverID)
	if status != DriverStatusBusy {
		t.Fatalf("expected BUSY, got %s", status)
	}

	// 2. Passenger abruptly cancels the trip
	cancelled, err := engine.CancelTrip(ctx, tripID)
	if err != nil || !cancelled {
		t.Fatalf("failed to cancel trip: %v", err)
	}

	// 3. Verify Trip is CANCELLED
	tripState, _, _ := engine.GetTripState(ctx, tripID)
	if tripState != TripStateCancelled {
		t.Fatalf("expected trip to be CANCELLED, got %s", tripState)
	}

	// 4. CRITICAL: Verify Driver was ATOMICALLY REVERTED to AVAILABLE (0 Zombie Drivers!)
	statusAfter, _ := engine.GetDriverStatus(ctx, driverID)
	if statusAfter != DriverStatusAvailable {
		t.Fatalf("ZOMBIE DRIVER DETECTED! Driver should be AVAILABLE after trip cancellation, got %s", statusAfter)
	}
	t.Logf("Compensating cancellation verified: driver %s successfully returned to AVAILABLE!", driverID)
}

func TestReaper_NeutralizesUnconfirmedTrips(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	engine := NewMatchingEngine(client)
	ctx := context.Background()

	const numTrips = 50
	for i := 0; i < numTrips; i++ {
		tripID := fmt.Sprintf("trip-unack-%d", i)
		driverID := fmt.Sprintf("driver-unack-%d", i)

		_ = engine.CreateTrip(ctx, tripID, "rider-test")
		_ = engine.SetDriverStatus(ctx, driverID, DriverStatusAvailable)
		_, _ = engine.AssignDriver(ctx, tripID, driverID)
	}

	// Simulate that the 15-second TTL expired by shifting the sorted set deadlines into the past
	pastTime := time.Now().Add(-20 * time.Second).Unix()
	for i := 0; i < numTrips; i++ {
		tripID := fmt.Sprintf("trip-unack-%d", i)
		client.ZAdd(ctx, TripDeadlinesZSetKey, redis.Z{
			Score:  float64(pastTime),
			Member: tripID,
		})
	}

	// Execute Reaper
	reapedCount, err := engine.ReapExpiredTrips(ctx)
	if err != nil {
		t.Fatalf("Reaper failed: %v", err)
	}

	if reapedCount != numTrips {
		t.Fatalf("expected %d trips reaped, got %d", numTrips, reapedCount)
	}

	// Verify all 50 drivers were saved from zombie state and returned to AVAILABLE!
	zombieDrivers := 0
	for i := 0; i < numTrips; i++ {
		driverID := fmt.Sprintf("driver-unack-%d", i)
		status, _ := engine.GetDriverStatus(ctx, driverID)
		if status != DriverStatusAvailable {
			zombieDrivers++
		}
	}

	if zombieDrivers > 0 {
		t.Fatalf("FAILED: %d zombie drivers detected!", zombieDrivers)
	}
	t.Logf("Reaper clean run: %d unconfirmed trips cancelled and 0 zombie drivers remaining!", numTrips)
}
