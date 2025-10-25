package entities

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Point struct {
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Description string  `json:"description,omitempty"`
}
type Route struct {
	ID     int     `json:"id"`
	Points []Point `json:"points"`
}
type RouteOutput struct {
	Routes []Route `json:"routes"`
}

type RouteInput struct {
	Origin      Coordinates
	Destination string
}
