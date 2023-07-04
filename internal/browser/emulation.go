package browser

import "github.com/go-rod/rod/lib/proto"

// BrowserGeolocation https://pkg.go.dev/github.com/go-rod/rod@v0.81.3/lib/proto#EmulationSetGeolocationOverride
type BrowserGeolocation struct {
	// Latitude (optional) Mock latitude
	Latitude float64 `json:"latitude,omitempty"`

	// Longitude (optional) Mock longitude
	Longitude float64 `json:"longitude,omitempty"`

	// Accuracy (optional) Mock accuracy
	Accuracy float64 `json:"accuracy,omitempty"`
}

type EmulationConfig struct {
	UserAgent           string
	OverrideGeolocation BrowserGeolocation
	Viewport            proto.EmulationSetDeviceMetricsOverride
}
