package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidWebSocketSource(t *testing.T) {
	for _, source := range []string{"Proxy", "Scanner", "Crawler", "Browser", "Repeater", SourcePlayground, SourceWsFuzz} {
		require.True(t, IsValidWebSocketSource(source), "%q is written to WebSocketConnection.Source", source)
	}

	for _, source := range []string{"proxy", "scanner", "PLAYGROUND", "ws-fuzz", "", "bogus"} {
		require.False(t, IsValidWebSocketSource(source), "%q is not a source value", source)
	}
}

func TestWebSocketSourcesDoesNotAliasSources(t *testing.T) {
	before := append([]string{}, Sources...)
	_ = append(WebSocketSources, "mutation")
	require.Equal(t, before, Sources, "appending to WebSocketSources must not write into the Sources backing array")
}
