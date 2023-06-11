package payloads

type PayloadInteractionData struct {
	InteractionDomain string
	InteractionFullID string
}

// PayloadInterface needed to implement if want to use the payloads with the fuzzer
type PayloadInterface interface {
	GetValue() string
	MatchAgainstString(string) (bool, error)
	GetInteractionData() PayloadInteractionData
}

type BasePayload struct{}

func (p BasePayload) GetInteractionData() PayloadInteractionData {
	return PayloadInteractionData{}
}
