package payloads

import (
	"regexp"
	"sukyan/lib"
)

type HostHeaderInjectionPayload struct {
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
		Value: lib.GenerateRandomString(10) + ".com",
	})
	payloads = append(payloads, HostHeaderInjectionPayload{
		Value: lib.GenerateRandomString(10) + "." + lib.GenerateRandomString(10) + ".com",
	})
	return payloads
}
