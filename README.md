# Google Maps Route API Service

A Go-based microservice that provides optimized cycling routes with elevation data and turn-by-turn navigation.

## Overview

This service accepts origin/destination coordinates and returns detailed cycling routes with:

- Turn-by-turn navigation instructions
- Street names 
- Elevation data
- Downhill segment detection
- Multiple route alternatives when available

## API Endpoint

### POST `/route`

Creates a new cycling route between two points.

#### Request Body

```json
{
  "Origin": {
    "lat": number,
    "lng": number
  },
  "Destination": string
}
```

#### Response

```json
{
  "routes": [
    {
      "id": number,
      "points": [
        {
          "lat": number,
          "lng": number,
          "description": string,
          "elevation": number,
          "is_down_hill": boolean
        }
      ]
    }
  ]
}
```

#### Response Fields

- `routes`: Array of available cycling routes
  - `id`: Unique identifier for the route
  - `points`: Array of navigation points along the route
    - `lat`: Latitude coordinate
    - `lng`: Longitude coordinate
    - `description`: Street name or turn instruction
    - `elevation`: Elevation in meters
    - `is_down_hill`: Indicates if this segment goes downhill