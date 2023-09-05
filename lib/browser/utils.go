package browser

import (
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
	"strings"
)

// ConvertToNetworkHeaders converts map[string][]string to NetworkHeaders
func ConvertToNetworkHeaders(headersMap map[string][]string) proto.NetworkHeaders {
	networkHeaders := make(proto.NetworkHeaders)
	for key, values := range headersMap {
		// Join multiple header values into a single string separated by commas
		combinedValues := strings.Join(values, ", ")
		networkHeaders[key] = gson.New(combinedValues)
	}
	return networkHeaders
}
