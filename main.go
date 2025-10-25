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
			Mode:        maps.TravelModeWalking, // Changed from Bicycling for better pedestrian path accuracy
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
			instructions := []entities.Instruction{}
			
			cumulativeDistance := 0
			cumulativeTime := 0

			for _, leg := range rt.Legs {
				var lastDesc string
				for _, step := range leg.Steps {
					lat := step.StartLocation.Lat
					lng := step.StartLocation.Lng

					// Extract instruction from Google
					htmlInst := step.HTMLInstructions
					distanceMeters := step.Distance.Meters
					durationSecs := int(step.Duration.Seconds())
					
					// Extract street name from HTML instruction
					streetName := extractStreetNameFromHTML(htmlInst)
					if streetName == "" {
						streetName = stripHTML(htmlInst)
					}
					
					// Build instruction object
					instruction := entities.Instruction{
						Instruction:     htmlInst,
						DistanceMeters:  cumulativeDistance,
						DurationSeconds: cumulativeTime,
						Maneuver:        "", // Google Maps Go library doesn't expose maneuver field
						StreetName:      streetName,
						StartLocation:   entities.Coordinates{Lat: lat, Lng: lng},
					}
					instructions = append(instructions, instruction)
					
					cumulativeDistance += distanceMeters
					cumulativeTime += durationSecs

					// Prefer clean street name from reverse geocode
					desc := extractStreetNameFromReverseGeocode(client, lat, lng)
					if desc == "" {
						desc = stripHTML(step.HTMLInstructions)
					}

					// Skip repeated or empty street names
					if desc == "" || desc == lastDesc {
						continue
					}
					lastDesc = desc

					elev, err := getElevation(client, lat, lng)
					if err != nil {
						elev = 0
					}

					points = append(points, entities.Point{
						Lat:         lat,
						Lng:         lng,
						Description: desc,
						Elevation:   elev,
						IsDownHill:  false,
					})
				}

				// Add final destination instruction
				endLat := leg.EndLocation.Lat
				endLng := leg.EndLocation.Lng
				endDesc := extractStreetNameFromReverseGeocode(client, endLat, endLng)
				if endDesc == "" {
					endDesc = "Destination"
				}
				
				instructions = append(instructions, entities.Instruction{
					Instruction:     "Arrive at " + endDesc,
					DistanceMeters:  cumulativeDistance,
					DurationSeconds: cumulativeTime,
					Maneuver:        "arrive",
					StreetName:      endDesc,
					StartLocation:   entities.Coordinates{Lat: endLat, Lng: endLng},
				})
				
				// Add final leg point
				elev, err := getElevation(client, endLat, endLng)
				if err != nil {
					elev = 0
				}
				points = append(points, entities.Point{
					Lat:         endLat,
					Lng:         endLng,
					Description: endDesc,
					Elevation:   elev,
					IsDownHill:  false,
				})
			}

			// Step 1: simplify close points (<50 m)
			simplified := simplifyRoute(points, 50.0)

			// Step 2: remove micro backtracks or “zig-zags”
			simplified = removeZigZags(simplified, 30.0)

			// Step 3: merge duplicates
			simplified = mergeDuplicateDescriptions(simplified)

			// Step 4: set downhill info
			for j := 0; j < len(simplified)-1; j++ {
				if simplified[j+1].Elevation < simplified[j].Elevation {
					simplified[j].IsDownHill = true
				}
			}

			route.Points = simplified
			route.Instructions = instructions
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

// =======================
// Utility Helper Functions
// =======================

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

// extractStreetNameFromReverseGeocode tries to get a clean street name
// and ignores Plus Codes or generic placeholders.
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
				name := comp.LongName
				if strings.Contains(name, "+") || strings.HasPrefix(name, "Unnamed") {
					return ""
				}
				return name
			}
		}
	}

	formatted := resp[0].FormattedAddress
	if strings.Contains(formatted, "+") || strings.Contains(formatted, "Unnamed") {
		return ""
	}
	return formatted
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

// extractStreetNameFromHTML parses street name from Google HTML instructions
// e.g., "Turn <b>left</b> onto <b>Market St</b>" -> "Market St"
func extractStreetNameFromHTML(html string) string {
	// Look for text in <b> tags that comes after "onto" or "on"
	lower := strings.ToLower(html)
	
	if idx := strings.Index(lower, " onto "); idx >= 0 {
		after := html[idx+6:]
		// Find first <b>...</b> after "onto"
		if start := strings.Index(after, "<b>"); start >= 0 {
			after = after[start+3:]
			if end := strings.Index(after, "</b>"); end >= 0 {
				return strings.TrimSpace(after[:end])
			}
		}
	}
	
	if idx := strings.Index(lower, " on "); idx >= 0 {
		after := html[idx+4:]
		if start := strings.Index(after, "<b>"); start >= 0 {
			after = after[start+3:]
			if end := strings.Index(after, "</b>"); end >= 0 {
				return strings.TrimSpace(after[:end])
			}
		}
	}
	
	// Fallback: strip all HTML and return
	return stripHTML(html)
}

// simplifyRoute removes points that are too close together (< minDist meters)
func simplifyRoute(points []entities.Point, minDist float64) []entities.Point {
	if len(points) <= 2 {
		return points
	}

	simplified := []entities.Point{points[0]} // keep first
	for i := 1; i < len(points)-1; i++ {
		last := simplified[len(simplified)-1]
		curr := points[i]
		dist := haversine(last.Lat, last.Lng, curr.Lat, curr.Lng)
		if dist >= minDist {
			simplified = append(simplified, curr)
		}
	}
	simplified = append(simplified, points[len(points)-1])
	return simplified
}

// removeZigZags removes small “back-and-forth” hops (<minBacktrack meters)
func removeZigZags(points []entities.Point, minBacktrack float64) []entities.Point {
	if len(points) < 3 {
		return points
	}

	cleaned := []entities.Point{points[0]}
	for i := 1; i < len(points)-1; i++ {
		prev := cleaned[len(cleaned)-1]
		curr := points[i]
		next := points[i+1]

		d1 := haversine(prev.Lat, prev.Lng, curr.Lat, curr.Lng)
		d2 := haversine(curr.Lat, curr.Lng, next.Lat, next.Lng)
		backtrack := haversine(prev.Lat, prev.Lng, next.Lat, next.Lng)

		// If the segment doubles back, skip curr
		if backtrack < d1 && backtrack < d2 && backtrack < minBacktrack {
			continue
		}
		cleaned = append(cleaned, curr)
	}

	cleaned = append(cleaned, points[len(points)-1])
	return cleaned
}

// mergeDuplicateDescriptions merges consecutive identical street names
func mergeDuplicateDescriptions(points []entities.Point) []entities.Point {
	if len(points) == 0 {
		return points
	}

	merged := []entities.Point{points[0]}
	for i := 1; i < len(points); i++ {
		if points[i].Description != merged[len(merged)-1].Description {
			merged = append(merged, points[i])
		}
	}
	return merged
}

// haversine returns distance in meters between two lat/lng points
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0 // Earth radius in meters
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
