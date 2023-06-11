package payloads

import (
	"github.com/pyneda/sukyan/lib/integrations"
	"regexp"
)

func GetSSRFProtocols() []string {
	return []string{
		"https://",
		"http://",
		"dict://",
		"file://",
		"ftp://",
		"gopher://",
		"imap://",
		"ldap://",
		"ldaps://",
	}
}

type SSRFPayload struct {
	Value             string
	InteractionDomain string
	InteractionFullID string
}

// GetValue gets the payload value as string
func (p SSRFPayload) GetValue() string {
	return p.Value
}

// MatchAgainstString Checks if the payload match against a string
func (p SSRFPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Value, text)
}

func (p SSRFPayload) GetInteractionData() PayloadInteractionData {
	return PayloadInteractionData{
		InteractionDomain: p.InteractionDomain,
		InteractionFullID: p.InteractionFullID,
	}
}

func GenerateSSRFPayloads(im *integrations.InteractionsManager) (payloads []PayloadInterface) {
	protocols := GetSSRFProtocols()
	for _, proto := range protocols {
		url := im.GetURL()
		value := proto + url.URL
		payload := SSRFPayload{
			Value:             value,
			InteractionDomain: url.URL,
			InteractionFullID: url.ID,
		}
		payloads = append(payloads, payload)
	}
	return payloads
}
