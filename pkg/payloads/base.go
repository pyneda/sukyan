package payloads

// PayloadInterface needed to implement if want to use the payloads with the fuzzer
type PayloadInterface interface {
	GetValue() string
	MatchAgainstString(string) (bool, error)
}
