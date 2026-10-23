package geo

// DriverLocation models a driver's geographic coordinate and optional distance from a search query.
type DriverLocation struct {
	DriverID   string  `json:"driver_id"`
	Longitude  float64 `json:"longitude"`
	Latitude   float64 `json:"latitude"`
	DistanceKm float64 `json:"distance_km,omitempty"`
}
