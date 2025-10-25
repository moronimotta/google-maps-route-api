package utils

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadConfig() string {
	envFile, _ := godotenv.Read(".env")

	envFileGoogleMapsApiKey := envFile["GOOGLE_MAPS_API_KEY"]
	apiKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if apiKey == "" {
		apiKey = envFileGoogleMapsApiKey
	}

	if apiKey == "" {
		log.Fatal("set GOOGLE_MAPS_API_KEY environment variable")
	}
	return apiKey
}
