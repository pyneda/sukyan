//go:build integration

package integrations

import (
	"context"
	"testing"
	"time"
)

// Opt-in, network-dependent integration test. Run with:
//
//	go test ./lib/integrations/ -tags integration -run TestVerifyOOBChannel -v
//
// It exercises the full out-of-band round-trip against the public interactsh
// servers: register -> self-callback -> poll -> decrypt. This is the regression
// guard for client/server protocol drift. It is exactly this round-trip that
// silently broke when the servers switched their AES mode from CFB to CTR while
// the client was pinned to a CFB-only release, disabling every blind/OOB check.
// If this test starts failing, the pinned interactsh client can no longer decrypt
// what the live servers emit and the dependency must be re-aligned.
func TestVerifyOOBChannel_LiveRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-dependent OOB round-trip in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	healthy, domain := VerifyOOBChannel(ctx, "", 45*time.Second)
	t.Logf("VerifyOOBChannel healthy=%v domain=%q", healthy, domain)
	if domain == "" {
		t.Fatal("VerifyOOBChannel returned an empty domain: interactsh registration failed (check outbound network to the interactsh servers)")
	}
	if !healthy {
		t.Fatalf("VerifyOOBChannel reported the OOB channel DORMANT for %q: the self-test callback was registered but never polled back and decrypted. The pinned interactsh client likely no longer matches the live server protocol.", domain)
	}
}
