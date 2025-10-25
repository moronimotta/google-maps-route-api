package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"test-module-maps/entities"
	"test-module-maps/utils"

	maps "googlemaps.github.io/maps"
)

func main() {

	apiKey := utils.LoadConfig()

	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
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
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		originStr := fmt.Sprintf("%f,%f", req.Origin.Lat, req.Origin.Lng)
		dr := &maps.DirectionsRequest{
			Origin:      originStr,
			Destination: req.Destination,
			Mode:        maps.TravelModeBicycling, // BICYCLING MODE
		}

		routesResp, _, err := client.Directions(context.Background(), dr)
		if err != nil {
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
						// fallback to plain text instruction (strip simple HTML)
						desc = stripHTML(step.HTMLInstructions)
					}
					points = append(points, entities.Point{Lat: lat, Lng: lng, Description: desc})
				}
				// include final end location of leg
				endLat := leg.EndLocation.Lat
				endLng := leg.EndLocation.Lng
				endDesc := extractStreetNameFromReverseGeocode(client, endLat, endLng)
				points = append(points, entities.Point{Lat: endLat, Lng: endLng, Description: endDesc})
			}
			route.Points = points
			out.Routes = append(out.Routes, route)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
		message := utils.FormatInfoNotification(fmt.Sprintf("Route request processed: Origin=%s, Destination=%s, RoutesFound=%d", originStr, req.Destination, len(out.Routes)), "Route Handler")
		utils.SendNotification(message)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func extractStreetNameFromReverseGeocode(client *maps.Client, lat, lng float64) string {
	resp, err := client.ReverseGeocode(context.Background(), &maps.GeocodingRequest{
		LatLng: &maps.LatLng{Lat: lat, Lng: lng},
	})
	if err != nil || len(resp) == 0 {
		return ""
	}
	// prefer the first result's address component with type "route"
	for _, comp := range resp[0].AddressComponents {
		for _, t := range comp.Types {
			if t == "route" {
				return comp.LongName
			}
		}
	}
	// fallback to formatted address
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
