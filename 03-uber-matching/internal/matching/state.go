package matching

import "time"

// TripState represents the finite state machine of a trip.
type TripState string

const (
	TripStatePending   TripState = "PENDING"
	TripStateMatched   TripState = "MATCHED"
	TripStateInTransit TripState = "IN_TRANSIT"
	TripStateCompleted TripState = "COMPLETED"
	TripStateCancelled TripState = "CANCELLED"
)

// DriverStatus models the real-time operational availability of a driver.
type DriverStatus string

const (
	DriverStatusAvailable DriverStatus = "AVAILABLE"
	DriverStatusBusy      DriverStatus = "BUSY"
	DriverStatusOffline   DriverStatus = "OFFLINE"
)

type Trip struct {
	TripID     string    `json:"trip_id"`
	RiderID    string    `json:"rider_id"`
	DriverID   string    `json:"driver_id,omitempty"`
	State      TripState `json:"state"`
	PickupLon  float64   `json:"pickup_lon"`
	PickupLat  float64   `json:"pickup_lat"`
	DropoffLon float64   `json:"dropoff_lon"`
	DropoffLat float64   `json:"dropoff_lat"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
