package main

import (
	"bike-router/entities"
	"bike-router/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"

	maps "googlemaps.github.io/maps"
)

func main() {

	apiKey := utils.LoadConfig()

	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		message := utils.FormatErrorNotification(fmt.Errorf("maps.NewClient: %v", err), "Main")
		utils.SendNotification(message)
		log.Fatalf("maps.NewClient: %v", err)
	}

	http.HandleFunc("/route", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			message := utils.FormatErrorNotification(fmt.Errorf("invalid method: %s", r.Method), "Route Handler")
			utils.SendNotification(message)
			http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
			return
		}

		var req entities.RouteInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			message := utils.FormatErrorNotification(fmt.Errorf("invalid json: %v", err), "Route Handler")
			utils.SendNotification(message)
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		originStr := fmt.Sprintf("%f,%f", req.Origin.Lat, req.Origin.Lng)
		dr := &maps.DirectionsRequest{
			Origin:      originStr,
			Destination: req.Destination,
			Mode:        maps.TravelModeBicycling,
		}

		routesResp, _, err := client.Directions(context.Background(), dr)
		if err != nil {
			message := utils.FormatErrorNotification(fmt.Errorf("directions error: %v", err), "Route Handler")
			utils.SendNotification(message)
			http.Error(w, "directions error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if len(routesResp) == 0 {
			http.Error(w, "no routes", http.StatusNotFound)
			return
		}

		out := entities.RouteOutput{Routes: make([]entities.Route, 0, len(routesResp))}
		for i, rt := range routesResp {
			route := entities.Route{ID: i + 1}
			points := []entities.Point{}
			for _, leg := range rt.Legs {
				for _, step := range leg.Steps {
					lat := step.StartLocation.Lat
					lng := step.StartLocation.Lng
					desc := extractStreetNameFromReverseGeocode(client, lat, lng)
					if desc == "" {
						desc = stripHTML(step.HTMLInstructions)
					}

					elev, err := getElevation(client, lat, lng)
					if err != nil {
						elev = 0
					}

					points = append(points, entities.Point{
						Lat:         lat,
						Lng:         lng,
						Description: desc,
						Elevation:   elev,
						IsDownHill:  false, // default false; will update later
					})
				}
				endLat := leg.EndLocation.Lat
				endLng := leg.EndLocation.Lng
				endDesc := extractStreetNameFromReverseGeocode(client, endLat, endLng)
				elev, err := getElevation(client, endLat, endLng)
				if err != nil {
					elev = 0
				}
				points = append(points, entities.Point{
					Lat:         endLat,
					Lng:         endLng,
					Description: endDesc,
					Elevation:   elev,
					IsDownHill:  false, // default false
				})
			}

			// Simplify route: remove waypoints too close together (< 15m apart)
			simplified := simplifyRoute(points, 15.0)

			// Update IsDownHill for each point except the last
			for j := 0; j < len(simplified)-1; j++ {
				if simplified[j+1].Elevation < simplified[j].Elevation {
					simplified[j].IsDownHill = true
				}
			}

			route.Points = simplified
			out.Routes = append(out.Routes, route)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)

		message := utils.FormatInfoNotification(
			fmt.Sprintf("Route request processed: Origin=%s, Destination=%s, RoutesFound=%d", originStr, req.Destination, len(out.Routes)),
			"Route Handler",
		)
		utils.SendNotification(message)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

// getElevation fetches elevation in meters for a given lat/lng
func getElevation(client *maps.Client, lat, lng float64) (float64, error) {
	resp, err := client.Elevation(context.Background(), &maps.ElevationRequest{
		Locations: []maps.LatLng{{Lat: lat, Lng: lng}},
	})
	if err != nil || len(resp) == 0 {
		return 0, err
	}
	return resp[0].Elevation, nil
}

func extractStreetNameFromReverseGeocode(client *maps.Client, lat, lng float64) string {
	resp, err := client.ReverseGeocode(context.Background(), &maps.GeocodingRequest{
		LatLng: &maps.LatLng{Lat: lat, Lng: lng},
	})
	if err != nil || len(resp) == 0 {
		return ""
	}
	for _, comp := range resp[0].AddressComponents {
		for _, t := range comp.Types {
			if t == "route" {
				return comp.LongName
			}
		}
	}
	return resp[0].FormattedAddress
}

func stripHTML(s string) string {
	out := make([]rune, 0, len(s))
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out = append(out, r)
		}
	}
	return strings.TrimSpace(string(out))
}

// simplifyRoute removes waypoints that are too close together (< minDist meters)
// Always keeps the first and last point
func simplifyRoute(points []entities.Point, minDist float64) []entities.Point {
	if len(points) <= 2 {
		return points
	}

	simplified := []entities.Point{points[0]} // always keep first

	for i := 1; i < len(points)-1; i++ {
		last := simplified[len(simplified)-1]
		curr := points[i]
		dist := haversine(last.Lat, last.Lng, curr.Lat, curr.Lng)

		// Keep this point if it's far enough from the last kept point
		if dist >= minDist {
			simplified = append(simplified, curr)
		}
	}

	// Always keep last point
	simplified = append(simplified, points[len(points)-1])
	return simplified
}

// haversine returns distance in meters between two lat/lng points
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0 // Earth radius in meters
	lat1Rad := lat1 * 3.14159265359 / 180.0
	lat2Rad := lat2 * 3.14159265359 / 180.0
	dLat := (lat2 - lat1) * 3.14159265359 / 180.0
	dLng := (lng2 - lng1) * 3.14159265359 / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
