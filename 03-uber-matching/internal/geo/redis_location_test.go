package geo

import (
	"context"
	"fmt"
	"math"
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

func TestSpatialPrecision(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewRedisLocationRepository(client, "test:drivers:geo")
	ctx := context.Background()

	// Madrid Landmark Coordinates:
	// Puerta del Sol: Lon: -3.703790, Lat: 40.416775
	// Plaza Mayor:    Lon: -3.707398, Lat: 40.415363
	puertaDelSolLon, puertaDelSolLat := -3.703790, 40.416775
	plazaMayorLon, plazaMayorLat := -3.707398, 40.415363

	// Insert driver at Plaza Mayor
	err := repo.UpsertDriverLocation(ctx, "driver-plaza-mayor", plazaMayorLon, plazaMayorLat)
	if err != nil {
		t.Fatalf("failed to upsert driver: %v", err)
	}

	// Search from Puerta del Sol with radius 1.0 km
	drivers, err := repo.FindDriversNear(ctx, puertaDelSolLon, puertaDelSolLat, 1.0, 5)
	if err != nil {
		t.Fatalf("failed to find drivers near Puerta del Sol: %v", err)
	}

	if len(drivers) != 1 {
		t.Fatalf("expected 1 driver found, got %d", len(drivers))
	}

	d := drivers[0]
	if d.DriverID != "driver-plaza-mayor" {
		t.Fatalf("expected driver-plaza-mayor, got %s", d.DriverID)
	}

	// Haversine geodesic distance between Sol and Plaza Mayor is ~0.344 km (344 meters)
	expectedDistKm := 0.344
	diff := math.Abs(d.DistanceKm - expectedDistKm)
	t.Logf("Calculated distance from Sol to Plaza Mayor: %.4f km (Difference from haversine: %.4f km)", d.DistanceKm, diff)

	if diff > 0.05 {
		t.Fatalf("distance error exceeds acceptable margin: got %.4f km, expected ~%.4f km", d.DistanceKm, expectedDistKm)
	}
}

func TestDriverConsistency(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewRedisLocationRepository(client, "test:drivers:geo")
	ctx := context.Background()

	driverID := "driver-persistent-42"
	baseLon, baseLat := -3.703790, 40.416775

	// Upsert 10 times with moving coordinates
	var finalLon, finalLat float64
	for i := 0; i < 10; i++ {
		finalLon = baseLon + float64(i)*0.001
		finalLat = baseLat + float64(i)*0.001
		if err := repo.UpsertDriverLocation(ctx, driverID, finalLon, finalLat); err != nil {
			t.Fatalf("failed to update location at step %d: %v", i, err)
		}
	}

	// Search in radius 5 km
	drivers, err := repo.FindDriversNear(ctx, baseLon, baseLat, 5.0, 100)
	if err != nil {
		t.Fatalf("failed to search drivers: %v", err)
	}

	// Must contain exactly 1 entry for this driver, never ghost duplicates!
	if len(drivers) != 1 {
		t.Fatalf("ghost duplicate leak! expected exactly 1 driver in index, found %d", len(drivers))
	}

	// Verify coordinates reflect the final 10th update
	lonDiff := math.Abs(drivers[0].Longitude - finalLon)
	latDiff := math.Abs(drivers[0].Latitude - finalLat)
	if lonDiff > 0.0001 || latDiff > 0.0001 {
		t.Fatalf("coordinates mismatch: expected (%.6f, %.6f), got (%.6f, %.6f)", finalLon, finalLat, drivers[0].Longitude, drivers[0].Latitude)
	}
}

func TestRemoveDriver(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewRedisLocationRepository(client, "test:drivers:geo")
	ctx := context.Background()

	_ = repo.UpsertDriverLocation(ctx, "driver-offline-1", -3.700000, 40.410000)

	drivers, _ := repo.FindDriversNear(ctx, -3.700000, 40.410000, 1.0, 10)
	if len(drivers) != 1 {
		t.Fatalf("expected driver to exist before removal")
	}

	// Remove driver (goes offline or assigned)
	if err := repo.RemoveDriver(ctx, "driver-offline-1"); err != nil {
		t.Fatalf("failed to remove driver: %v", err)
	}

	driversAfter, _ := repo.FindDriversNear(ctx, -3.700000, 40.410000, 1.0, 10)
	if len(driversAfter) != 0 {
		t.Fatalf("expected 0 drivers after removal, got %d", len(driversAfter))
	}
}

func BenchmarkPipelineVsSingle(b *testing.B) {
	client, cleanup := setupTestRedis(nil)
	defer cleanup()

	repo := NewRedisLocationRepository(client, "bench:drivers:geo")
	ctx := context.Background()

	const numDrivers = 1000
	locations := make([]DriverLocation, numDrivers)
	for i := 0; i < numDrivers; i++ {
		locations[i] = DriverLocation{
			DriverID:  fmt.Sprintf("driver-%d", i),
			Longitude: -3.703790 + float64(i)*0.0001,
			Latitude:  40.416775 + float64(i)*0.0001,
		}
	}

	b.Run("SingleRoundTrip_1000", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, loc := range locations {
				_ = repo.UpsertDriverLocation(ctx, loc.DriverID, loc.Longitude, loc.Latitude)
			}
		}
	})

	b.Run("PipelinedBatch_1000", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = repo.UpsertDriverLocationBatchBenchmark(ctx, locations)
		}
	})
}

// Helper to benchmark pipeline specifically
func (r *RedisLocationRepository) UpsertDriverLocationBatchBenchmark(ctx context.Context, locations []DriverLocation) error {
	return r.UpsertDriverLocationsBatch(ctx, locations)
}
