package ui

import (
	"fmt"
	"math"
)

const earthRadiusKM = 6371.0

// HasCoordinates reports whether lat/lon are present (not both zero).
func HasCoordinates(lat, lon float64) bool {
	return lat != 0 || lon != 0
}

// DistanceKM returns the great-circle distance in kilometres between two points.
func DistanceKM(lat1, lon1, lat2, lon2 float64) float64 {
	const deg = math.Pi / 180
	lat1Rad := lat1 * deg
	lat2Rad := lat2 * deg
	dLat := (lat2 - lat1) * deg
	dLon := (lon2 - lon1) * deg

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKM * c
}

// FormatDistance renders a compact human distance label.
func FormatDistance(km float64) string {
	if km < 0 || math.IsNaN(km) {
		return "-"
	}
	switch {
	case km == 0:
		return "0 m"
	case km < 1:
		m := int(math.Round(km * 1000))
		if m < 1 {
			m = 1
		}
		return fmt.Sprintf("%d m", m)
	case km < 10:
		return fmt.Sprintf("%.1f km", km)
	default:
		return fmt.Sprintf("%d km", int(math.Round(km)))
	}
}
