package geo

import (
	"context"
	"fmt"
	"testing"
)

func TestRadiusStepping(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewRedisLocationRepository(client, "test:stepping:geo")
	ctx := context.Background()

	// Sol coordinates: -3.703790, 40.416775
	solLon, solLat := -3.703790, 40.416775

	// Insert 10 close drivers within 400 meters (0.4 km)
	for i := 0; i < 10; i++ {
		driverID := fmt.Sprintf("driver-close-%d", i)
		_ = repo.UpsertDriverLocation(ctx, driverID, solLon+float64(i)*0.0002, solLat+float64(i)*0.0002)
	}

	// Insert 50 far drivers at 3.5 km
	for i := 0; i < 50; i++ {
		driverID := fmt.Sprintf("driver-far-%d", i)
		_ = repo.UpsertDriverLocation(ctx, driverID, solLon+0.03+float64(i)*0.001, solLat+0.03+float64(i)*0.001)
	}

	// Search requesting 5 drivers using progressive rings (500m -> 1km -> 2km -> 5km)
	drivers, err := repo.FindDriversNearStepping(ctx, solLon, solLat, 5, nil)
	if err != nil {
		t.Fatalf("failed Radius Stepping query: %v", err)
	}

	if len(drivers) != 5 {
		t.Fatalf("expected 5 drivers, got %d", len(drivers))
	}

	// Verify all returned drivers were resolved in the 500m ring (< 0.5 km)
	for _, d := range drivers {
		if d.DistanceKm > 0.5 {
			t.Fatalf("Radius Stepping failed to cap query! Returned driver at %.3f km, expected <= 0.5 km", d.DistanceKm)
		}
	}
	t.Logf("Radius Stepping satisfied query in the tight 500m ring with 0 unnecessary scans of the 50 far drivers!")
}
