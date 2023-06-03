package payloads

import "testing"

func TestTemplateLanguagePayload(t *testing.T) {
	payload := TemplateLanguagePayload{
		Value: "{{8263+8263}}",
		Regex: "16526",
	}
	match, err := payload.MatchAgainstString("<html><div>Payload result: 16526\n</div></html>")
	if err != nil {
		t.Error()
	}

	if !match {
		t.Error()
	}
	match, err = payload.MatchAgainstString("<html><div>Payload result: 12526\n</div></html>")
	if err != nil {
		t.Error()
	}

	if match {
		t.Error()
	}

}
