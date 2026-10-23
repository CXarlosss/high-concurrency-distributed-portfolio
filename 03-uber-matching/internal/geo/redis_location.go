package geo

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultDriversGeoKey = "drivers:geo"
	DefaultTimeout       = 100 * time.Millisecond
)

type RedisLocationRepository struct {
	client *redis.Client
	geoKey string
}

func NewRedisLocationRepository(client *redis.Client, geoKey string) *RedisLocationRepository {
	if geoKey == "" {
		geoKey = DefaultDriversGeoKey
	}
	return &RedisLocationRepository{
		client: client,
		geoKey: geoKey,
	}
}

// UpsertDriverLocation inserts or updates a driver's GPS coordinates using GEOADD.
// Since Redis GEO is backed by a 52-bit Geohash sorted set, updates overwrite the previous coordinate.
func (r *RedisLocationRepository) UpsertDriverLocation(ctx context.Context, driverID string, lon, lat float64) error {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	cmd := r.client.GeoAdd(reqCtx, r.geoKey, &redis.GeoLocation{
		Name:      driverID,
		Longitude: lon,
		Latitude:  lat,
	})
	if err := cmd.Err(); err != nil {
		return fmt.Errorf("failed GEOADD for driver %s: %w", driverID, err)
	}
	return nil
}

// RemoveDriver removes a driver from the geospatial index (e.g. when going offline or taking a trip).
// Since Redis stores GEO members in a ZSet, ZREM is the canonical O(log N) deletion operation.
func (r *RedisLocationRepository) RemoveDriver(ctx context.Context, driverID string) error {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	if err := r.client.ZRem(reqCtx, r.geoKey, driverID).Err(); err != nil {
		return fmt.Errorf("failed ZREM for driver %s: %w", driverID, err)
	}
	return nil
}

// FindDriversNear searches for the closest N drivers to a target (lon, lat) within a radius in kilometers.
// Uses GEOSEARCH with ASC ordering (closest first), returning exact distance and coordinates.
func (r *RedisLocationRepository) FindDriversNear(ctx context.Context, lon, lat, radiusKm float64, limit int) ([]DriverLocation, error) {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	query := &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  lon,
			Latitude:   lat,
			Radius:     radiusKm,
			RadiusUnit: "km",
			Sort:       "ASC",
			Count:      limit,
		},
		WithCoord: true,
		WithDist:  true,
	}

	results, err := r.client.GeoSearchLocation(reqCtx, r.geoKey, query).Result()
	if err != nil {
		return nil, fmt.Errorf("failed GEOSEARCH: %w", err)
	}

	drivers := make([]DriverLocation, len(results))
	for i, res := range results {
		drivers[i] = DriverLocation{
			DriverID:   res.Name,
			Longitude:  res.Longitude,
			Latitude:   res.Latitude,
			DistanceKm: res.Dist,
		}
	}
	return drivers, nil
}

// DefaultSteppingRadiiKm defines progressive search rings: 500m -> 1km -> 2km -> 5km.
var DefaultSteppingRadiiKm = []float64{0.5, 1.0, 2.0, 5.0}

// FindDriversNearStepping executes progressive ring searches (Radius Stepping).
// In high-density urban areas, it satisfies the query in the 500m ring with COUNT limit,
// avoiding O(N log N) sorting overhead of thousands of drivers across 5km in Redis.
func (r *RedisLocationRepository) FindDriversNearStepping(ctx context.Context, lon, lat float64, limit int, rings []float64) ([]DriverLocation, error) {
	if len(rings) == 0 {
		rings = DefaultSteppingRadiiKm
	}

	for _, radiusKm := range rings {
		drivers, err := r.FindDriversNear(ctx, lon, lat, radiusKm, limit)
		if err != nil {
			return nil, err
		}
		if len(drivers) >= limit {
			return drivers, nil
		}
		if len(drivers) > 0 && radiusKm >= 1.0 {
			// Found sufficient candidates in acceptable distance
			return drivers, nil
		}
	}
	return []DriverLocation{}, nil
}

// UpsertDriverLocationsBatch performs high-throughput bulk ingestion using Redis Pipelines.
// Multiple GEOADD commands are packaged into a single network round-trip.
func (r *RedisLocationRepository) UpsertDriverLocationsBatch(ctx context.Context, locations []DriverLocation) error {
	if len(locations) == 0 {
		return nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	pipe := r.client.Pipeline()
	for _, loc := range locations {
		pipe.GeoAdd(reqCtx, r.geoKey, &redis.GeoLocation{
			Name:      loc.DriverID,
			Longitude: loc.Longitude,
			Latitude:  loc.Latitude,
		})
	}

	_, err := pipe.Exec(reqCtx)
	if err != nil {
		return fmt.Errorf("failed pipelined GEOADD batch: %w", err)
	}
	return nil
}
