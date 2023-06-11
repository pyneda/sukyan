package payloads

import (
	"github.com/pyneda/sukyan/lib"
	"regexp"
)

type HostHeaderInjectionPayload struct {
	BasePayload
	Value string
}

// GetValue gets the payload value as string
func (p HostHeaderInjectionPayload) GetValue() string {
	return p.Value
}

// MatchAgainstString Checks if the payload match against a string
func (p HostHeaderInjectionPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Value, text)
}

func GetHostHeaderInjectionPayloads() (payloads []PayloadInterface) {
	payloads = append(payloads, HostHeaderInjectionPayload{
		Value: lib.GenerateRandomLowercaseString(10) + ".com",
	})
	payloads = append(payloads, HostHeaderInjectionPayload{
		Value: lib.GenerateRandomLowercaseString(10) + "." + lib.GenerateRandomLowercaseString(10) + ".com",
	})
	return payloads
}
