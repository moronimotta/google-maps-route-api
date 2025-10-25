package entities

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Point struct {
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Description string  `json:"description,omitempty"`
	Elevation   float64 `json:"elevation"` // meters
	IsDownHill  bool    `json:"is_down_hill"`
}

type Instruction struct {
	Instruction      string  `json:"instruction"`       // HTML instruction from Google (e.g., "Turn <b>left</b> onto Market St")
	DistanceMeters   int     `json:"distance_meters"`   // Distance from start to this instruction
	DurationSeconds  int     `json:"duration_seconds"`  // Time from start to this instruction
	Maneuver         string  `json:"maneuver"`          // turn-left, turn-right, straight, etc.
	StreetName       string  `json:"street_name"`       // Extracted street name
	StartLocation    Coordinates `json:"start_location"`
}

type Route struct {
	ID           int           `json:"id"`
	Points       []Point       `json:"points"`        // Simplified route polyline for map display
	Instructions []Instruction `json:"instructions"`  // Turn-by-turn instructions
}

type RouteOutput struct {
	Routes []Route `json:"routes"`
}

type RouteInput struct {
	Origin      Coordinates
	Destination string
}
