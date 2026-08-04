package api

import (
	"testing"

	"github.com/spf13/viper"
)

func TestBuildAPICapabilitiesReportsBodyLimit(t *testing.T) {
	viper.Set("api.body_limit", 32*1024*1024)
	defer viper.Set("api.body_limit", nil)

	capabilities := BuildAPICapabilities(APIServerOptions{EnableProxyServices: true})

	if capabilities.Limits.BodyLimit != 32*1024*1024 {
		t.Fatalf("BodyLimit = %d, want %d", capabilities.Limits.BodyLimit, 32*1024*1024)
	}
	if !capabilities.Features.ProxyServices.Enabled {
		t.Fatal("ProxyServices.Enabled = false, want true")
	}
}
