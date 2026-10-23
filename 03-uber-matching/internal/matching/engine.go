package matching

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TripKeyPrefix           = "trip:"
	DriverKeyPrefix         = "driver:"
	DefaultTripEventsStream = "trips:events"
	TripDeadlinesZSetKey    = "trip:ack_deadlines"
	DefaultTimeout          = 100 * time.Millisecond
	DefaultAckTimeoutSec    = 15
)

// assignDriverLuaScript guarantees 100% atomic state transitions, event-sourcing, and unconfirmed deadline tracking:
// - KEYS[1]: trip:{tripID}:state
// - KEYS[2]: driver:{driverID}:status
// - KEYS[3]: trip:{tripID}:driver
// - KEYS[4]: trips:events (Redis Stream)
// - KEYS[5]: trip:ack_deadlines (ZSet for Reaper TTL)
// - ARGV[1]: driver_id
// - ARGV[2]: trip_id
// - ARGV[3]: timestamp
// - ARGV[4]: deadline_timestamp
var assignDriverLuaScript = redis.NewScript(`
local trip_state = redis.call('GET', KEYS[1])
if trip_state ~= 'PENDING' then
    return 0
end

local driver_status = redis.call('GET', KEYS[2])
if driver_status ~= 'AVAILABLE' then
    return 0
end

redis.call('SET', KEYS[1], 'MATCHED')
redis.call('SET', KEYS[2], 'BUSY')
redis.call('SET', KEYS[3], ARGV[1])

-- Record deadline in Sorted Set for anti-zombie Reaper
redis.call('ZADD', KEYS[5], ARGV[4], ARGV[2])

-- Emit transactional event into Redis Stream
redis.call('XADD', KEYS[4], '*', 'trip_id', ARGV[2], 'driver_id', ARGV[1], 'state', 'MATCHED', 'timestamp', ARGV[3])

return 1
`)

// cancelTripLuaScript executes bidirectional atomic compensation:
// If passenger cancels a MATCHED trip, the driver is automatically and atomically reverted to AVAILABLE!
// - KEYS[1]: trip:{tripID}:state
// - KEYS[2]: trip:{tripID}:driver
// - KEYS[3]: trip:ack_deadlines
// - KEYS[4]: trips:events
// - ARGV[1]: trip_id
// - ARGV[2]: timestamp
var cancelTripLuaScript = redis.NewScript(`
local state = redis.call('GET', KEYS[1])
if not state or state == 'COMPLETED' or state == 'CANCELLED' then
    return 0
end

redis.call('ZREM', KEYS[3], ARGV[1])

if state == 'PENDING' then
    redis.call('SET', KEYS[1], 'CANCELLED')
    redis.call('XADD', KEYS[4], '*', 'trip_id', ARGV[1], 'state', 'CANCELLED', 'timestamp', ARGV[2])
    return 1
end

if state == 'MATCHED' then
    local assignedDriver = redis.call('GET', KEYS[2])
    redis.call('SET', KEYS[1], 'CANCELLED')
    if assignedDriver then
        local driverKey = 'driver:' .. assignedDriver .. ':status'
        redis.call('SET', driverKey, 'AVAILABLE')
        redis.call('DEL', KEYS[2])
    end
    redis.call('XADD', KEYS[4], '*', 'trip_id', ARGV[1], 'driver_id', assignedDriver or '', 'state', 'CANCELLED', 'timestamp', ARGV[2])
    return 2
end

return 0
`)

// reapTripLuaScript neutralizes zombie trips where the driver never confirmed within the TTL:
// - KEYS[1]: trip:{tripID}:state
// - KEYS[2]: trip:{tripID}:driver
// - KEYS[3]: trip:ack_deadlines
// - KEYS[4]: trips:events
// - ARGV[1]: trip_id
// - ARGV[2]: timestamp
var reapTripLuaScript = redis.NewScript(`
redis.call('ZREM', KEYS[3], ARGV[1])
local state = redis.call('GET', KEYS[1])
if state == 'MATCHED' then
    local driver = redis.call('GET', KEYS[2])
    redis.call('SET', KEYS[1], 'CANCELLED')
    if driver then
        local driverKey = 'driver:' .. driver .. ':status'
        redis.call('SET', driverKey, 'AVAILABLE')
        redis.call('DEL', KEYS[2])
    end
    redis.call('XADD', KEYS[4], '*', 'trip_id', ARGV[1], 'driver_id', driver or '', 'state', 'REAPED', 'timestamp', ARGV[2])
    return 1
end
return 0
`)

// confirmTripLuaScript marks trip as IN_TRANSIT and removes the TTL deadline:
// - KEYS[1]: trip:{tripID}:state
// - KEYS[2]: trip:ack_deadlines
// - ARGV[1]: trip_id
var confirmTripLuaScript = redis.NewScript(`
local state = redis.call('GET', KEYS[1])
if state == 'MATCHED' then
    redis.call('SET', KEYS[1], 'IN_TRANSIT')
    redis.call('ZREM', KEYS[2], ARGV[1])
    return 1
end
return 0
`)

type MatchingEngine struct {
	client *redis.Client
}

func NewMatchingEngine(client *redis.Client) *MatchingEngine {
	return &MatchingEngine{client: client}
}

func tripStateKey(tripID string) string {
	return fmt.Sprintf("%s%s:state", TripKeyPrefix, tripID)
}

func tripDriverKey(tripID string) string {
	return fmt.Sprintf("%s%s:driver", TripKeyPrefix, tripID)
}

func driverStatusKey(driverID string) string {
	return fmt.Sprintf("%s%s:status", DriverKeyPrefix, driverID)
}

// CreateTrip registers a new trip in PENDING state.
func (m *MatchingEngine) CreateTrip(ctx context.Context, tripID, riderID string) error {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	key := tripStateKey(tripID)
	// Only set if does not exist (NX)
	set, err := m.client.SetNX(reqCtx, key, string(TripStatePending), 1*time.Hour).Result()
	if err != nil {
		return fmt.Errorf("failed to create trip: %w", err)
	}
	if !set {
		return fmt.Errorf("trip %s already exists", tripID)
	}
	return nil
}

// SetDriverStatus updates the operational status of a driver.
func (m *MatchingEngine) SetDriverStatus(ctx context.Context, driverID string, status DriverStatus) error {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	key := driverStatusKey(driverID)
	return m.client.Set(reqCtx, key, string(status), 24*time.Hour).Err()
}

// GetTripState returns the current state and assigned driver (if any).
func (m *MatchingEngine) GetTripState(ctx context.Context, tripID string) (TripState, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	state, err := m.client.Get(reqCtx, tripStateKey(tripID)).Result()
	if err != nil {
		return "", "", err
	}

	driver, _ := m.client.Get(reqCtx, tripDriverKey(tripID)).Result()
	return TripState(state), driver, nil
}

// GetDriverStatus returns the current status of a driver.
func (m *MatchingEngine) GetDriverStatus(ctx context.Context, driverID string) (DriverStatus, error) {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	status, err := m.client.Get(reqCtx, driverStatusKey(driverID)).Result()
	if err != nil {
		return "", err
	}
	return DriverStatus(status), nil
}

// AssignDriver executes the atomic Lua script using EVALSHA.
// Returns true if the driver was atomically paired with the trip, or false if either
// the trip was already matched/cancelled or the driver was already claimed by another trip.
func (m *MatchingEngine) AssignDriver(ctx context.Context, tripID, driverID string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	now := time.Now()
	deadline := now.Add(time.Duration(DefaultAckTimeoutSec) * time.Second).Unix()

	keys := []string{
		tripStateKey(tripID),
		driverStatusKey(driverID),
		tripDriverKey(tripID),
		DefaultTripEventsStream,
		TripDeadlinesZSetKey,
	}
	args := []any{
		driverID,
		tripID,
		now.UnixNano(),
		deadline,
	}

	res, err := assignDriverLuaScript.Run(reqCtx, m.client, keys, args...).Int64()
	if err != nil {
		return false, fmt.Errorf("error executing atomic Lua matching: %w", err)
	}

	return res == 1, nil
}

// CancelTrip cancels a trip atomically. If the trip was MATCHED, it automatically frees
// the driver back to AVAILABLE in the same Redis execution cycle, preventing zombie drivers!
func (m *MatchingEngine) CancelTrip(ctx context.Context, tripID string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	keys := []string{
		tripStateKey(tripID),
		tripDriverKey(tripID),
		TripDeadlinesZSetKey,
		DefaultTripEventsStream,
	}
	args := []any{
		tripID,
		time.Now().UnixNano(),
	}

	res, err := cancelTripLuaScript.Run(reqCtx, m.client, keys, args...).Int64()
	if err != nil {
		return false, fmt.Errorf("error executing atomic trip cancellation: %w", err)
	}

	return res > 0, nil
}

// ConfirmTrip is invoked when the driver confirms acceptance, transitioning to IN_TRANSIT
// and clearing the unconfirmed deadline TTL.
func (m *MatchingEngine) ConfirmTrip(ctx context.Context, tripID string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	keys := []string{
		tripStateKey(tripID),
		TripDeadlinesZSetKey,
	}
	args := []any{tripID}

	res, err := confirmTripLuaScript.Run(reqCtx, m.client, keys, args...).Int64()
	if err != nil {
		return false, fmt.Errorf("error confirming trip: %w", err)
	}

	return res == 1, nil
}

// ReapExpiredTrips finds trips whose confirmation TTL expired, cancels them and frees the drivers.
func (m *MatchingEngine) ReapExpiredTrips(ctx context.Context) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	now := time.Now().Unix()
	// Find all trip IDs where deadline <= now
	expiredTripIDs, err := m.client.ZRangeByScore(reqCtx, TripDeadlinesZSetKey, &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		return 0, err
	}

	reaped := 0
	for _, tripID := range expiredTripIDs {
		keys := []string{
			tripStateKey(tripID),
			tripDriverKey(tripID),
			TripDeadlinesZSetKey,
			DefaultTripEventsStream,
		}
		args := []any{
			tripID,
			time.Now().UnixNano(),
		}

		res, err := reapTripLuaScript.Run(reqCtx, m.client, keys, args...).Int64()
		if err == nil && res == 1 {
			reaped++
		}
	}

	return reaped, nil
}
