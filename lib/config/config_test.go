package config

import (
	"testing"

	"github.com/spf13/viper"
)

// Secrets have to be settable without writing them to a file on disk, so every
// key doubles as SUKYAN_<KEY> with dots replaced by underscores.
func TestLoadConfigReadsSecretsFromTheEnvironment(t *testing.T) {
	t.Cleanup(viper.Reset)
	t.Setenv("SUKYAN_API_DASHBOARD_BASIC_AUTH_PASSWORD", "from-env")
	t.Setenv("SUKYAN_API_AUTH_JWT_SECRET_KEY", "env-secret")

	LoadConfig()

	for _, tc := range []struct{ key, want string }{
		{"api.dashboard.basic_auth.password", "from-env"},
		{"api.auth.jwt_secret_key", "env-secret"},
	} {
		if got := viper.GetString(tc.key); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}
}

// Shipping a known dashboard password would leave every default deployment open.
func TestDashboardPasswordHasNoDefault(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Reset()

	SetDefaultConfig()

	if got := viper.GetString("api.dashboard.basic_auth.password"); got != "" {
		t.Errorf("api.dashboard.basic_auth.password defaults to %q, want it unset", got)
	}
}
