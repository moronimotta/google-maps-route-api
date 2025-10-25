package main

import (
	"bike-router/entities"
	"bike-router/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

			// Update IsDownHill for each point except the last
			for j := 0; j < len(points)-1; j++ {
				if points[j+1].Elevation < points[j].Elevation {
					points[j].IsDownHill = true
				}
			}

			route.Points = points
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
