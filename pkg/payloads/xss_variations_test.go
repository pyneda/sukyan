package payloads

import (
	"strings"
	"testing"
)

func TestGenerateDialogVariations(t *testing.T) {
	base := XSSPayload{
		Value:         `<img src=x onerror=alert(1)>`,
		Categories:    []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
		EventType:     "onerror",
		RequiredChars: []string{"<", ">", "="},
		Confidence:    90,
	}

	variations := generateDialogVariations(base)

	// Should generate confirm, prompt, and print variations
	if len(variations) != 3 {
		t.Errorf("Expected 3 variations, got %d", len(variations))
	}

	hasConfirm := false
	hasPrompt := false
	hasPrint := false
	for _, v := range variations {
		if strings.Contains(v.Value, "confirm") {
			hasConfirm = true
		}
		if strings.Contains(v.Value, "prompt") {
			hasPrompt = true
		}
		if strings.Contains(v.Value, "print") {
			hasPrint = true
		}
	}

	if !hasConfirm {
		t.Error("Expected confirm variation")
	}
	if !hasPrompt {
		t.Error("Expected prompt variation")
	}
	if !hasPrint {
		t.Error("Expected print variation")
	}
}

func TestGenerateDialogVariations_NoAlert(t *testing.T) {
	base := XSSPayload{
		Value:      `<img src=x onerror=confirm(1)>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	variations := generateDialogVariations(base)

	// Should not generate variations since no "alert" in payload
	if len(variations) != 0 {
		t.Errorf("Expected 0 variations for non-alert payload, got %d", len(variations))
	}
}

func TestGenerateCaseMixVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `<script>alert(1)</script>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 95,
	}

	variations := generateCaseMixVariations(base)

	// Should generate at least one case-mixed variation
	if len(variations) == 0 {
		t.Error("Expected at least one case-mixed variation")
	}

	// Check that variation has different case
	foundCaseMix := false
	for _, v := range variations {
		if strings.Contains(v.Value, "ScRiPt") || strings.Contains(v.Value, "aLeRt") {
			foundCaseMix = true
			break
		}
	}
	if !foundCaseMix {
		t.Error("Expected case-mixed variation")
	}
}

func TestGenerateWhitespaceVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `<img src=x onerror=alert(1)>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	variations := generateWhitespaceVariations(base)

	// Should generate whitespace variations
	if len(variations) == 0 {
		t.Error("Expected at least one whitespace variation")
	}

	// Check for tab or newline encoding
	foundWhitespace := false
	for _, v := range variations {
		if strings.Contains(v.Value, "%09") || strings.Contains(v.Value, "%0a") {
			foundWhitespace = true
			break
		}
	}
	if !foundWhitespace {
		t.Error("Expected whitespace-encoded variation")
	}
}

func TestGenerateTagSlashVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `<img src=x onerror=alert(1)>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	variations := generateTagSlashVariations(base)

	// Should generate slash separator variation
	if len(variations) == 0 {
		t.Error("Expected slash separator variation")
	}

	foundSlash := false
	for _, v := range variations {
		if strings.Contains(v.Value, "<img/") {
			foundSlash = true
			break
		}
	}
	if !foundSlash {
		t.Error("Expected <img/src variation")
	}
}

func TestGenerateBacktickVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `<img src=x onerror=alert(1)>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	variations := generateBacktickVariations(base)

	// Should generate backtick call variation
	if len(variations) == 0 {
		t.Error("Expected backtick call variation")
	}

	foundBacktick := false
	for _, v := range variations {
		if strings.Contains(v.Value, "alert`1`") {
			foundBacktick = true
			break
		}
	}
	if !foundBacktick {
		t.Error("Expected alert`1` variation")
	}
}

func TestContainsFunctionCall(t *testing.T) {
	tests := []struct {
		payload  string
		expected bool
	}{
		{"alert(1)", true},
		{"confirm(1)", true},
		{"prompt('test')", true},
		{"eval('code')", true},
		{"<script>alert(1)</script>", true},
		{"onerror=alert(1)", true},
		{"<img src=x>", false},
		{"javascript:void(0)", false},
		{"alert`1`", false}, // backtick call is different
	}

	for _, tt := range tests {
		t.Run(tt.payload, func(t *testing.T) {
			result := containsFunctionCall(tt.payload)
			if result != tt.expected {
				t.Errorf("containsFunctionCall(%q) = %v, want %v", tt.payload, result, tt.expected)
			}
		})
	}
}

func TestGenerateUnicodeVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `alert(1)`,
		Categories: []XSSPayloadCategory{CategoryJSBreaking},
		Confidence: 90,
	}

	variations := generateUnicodeVariations(base)

	// Should generate unicode escape variation
	if len(variations) == 0 {
		t.Error("Expected unicode variation")
	}

	foundUnicode := false
	for _, v := range variations {
		if strings.Contains(v.Value, `\u0061`) { // unicode for 'a'
			foundUnicode = true
			break
		}
	}
	if !foundUnicode {
		t.Error("Expected \\u0061lert variation")
	}
}

func TestGenerateURLEncodedVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `<script>alert(1)</script>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	variations := generateURLEncodedVariations(base)

	// Should generate URL encoded variation
	if len(variations) == 0 {
		t.Error("Expected URL encoded variation")
	}

	foundEncoded := false
	for _, v := range variations {
		if strings.Contains(v.Value, "%3C") && strings.Contains(v.Value, "%3E") {
			foundEncoded = true
			break
		}
	}
	if !foundEncoded {
		t.Error("Expected %3C and %3E in URL encoded variation")
	}
}

func TestGenerateDoubleEncodedVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `<script>alert(1)</script>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	variations := generateDoubleEncodedVariations(base)

	// Should generate double encoded variation
	if len(variations) == 0 {
		t.Error("Expected double encoded variation")
	}

	foundDoubleEncoded := false
	for _, v := range variations {
		if strings.Contains(v.Value, "%253C") && strings.Contains(v.Value, "%253E") {
			foundDoubleEncoded = true
			break
		}
	}
	if !foundDoubleEncoded {
		t.Error("Expected %253C and %253E in double encoded variation")
	}
}

func TestGenerateStringConcatVariations(t *testing.T) {
	base := XSSPayload{
		Value:      `';alert(1);//`,
		Categories: []XSSPayloadCategory{CategoryJSBreaking},
		Confidence: 90,
	}

	variations := generateStringConcatVariations(base)

	// Should generate string concatenation variation
	if len(variations) == 0 {
		t.Error("Expected string concatenation variation")
	}

	foundConcat := false
	for _, v := range variations {
		if strings.Contains(v.Value, "window[") && strings.Contains(v.Value, "'+") {
			foundConcat = true
			break
		}
	}
	if !foundConcat {
		t.Error("Expected window['al'+'ert'] style variation")
	}
}

func TestGenerateVariations_AllEnabled(t *testing.T) {
	base := XSSPayload{
		Value:         `<img src=x onerror=alert(1)>`,
		Categories:    []XSSPayloadCategory{CategoryTagInjection, CategoryEventHandler},
		EventType:     "onerror",
		RequiredChars: []string{"<", ">", "="},
		Confidence:    90,
	}

	config := VariationConfig{
		EnableCaseMix:        true,
		EnableDialogFunction: true,
		EnableWhitespace:     true,
		EnableTagSlash:       true,
		EnableBacktickCall:   true,
		MaxVariations:        0, // Unlimited
	}

	variations := GenerateVariations(base, config)

	// Should have original plus multiple variations
	if len(variations) <= 1 {
		t.Errorf("Expected multiple variations, got %d", len(variations))
	}

	// First should be original
	if variations[0].Value != base.Value {
		t.Error("First variation should be original payload")
	}
}

func TestGenerateVariations_MaxLimit(t *testing.T) {
	base := XSSPayload{
		Value:      `<img src=x onerror=alert(1)>`,
		Categories: []XSSPayloadCategory{CategoryTagInjection},
		Confidence: 90,
	}

	config := VariationConfig{
		EnableCaseMix:        true,
		EnableDialogFunction: true,
		EnableWhitespace:     true,
		EnableTagSlash:       true,
		EnableBacktickCall:   true,
		MaxVariations:        5,
	}

	variations := GenerateVariations(base, config)

	if len(variations) > 5 {
		t.Errorf("Expected max 5 variations, got %d", len(variations))
	}
}

func TestDeduplicatePayloads(t *testing.T) {
	payloads := []XSSPayload{
		{Value: "alert(1)", Confidence: 90},
		{Value: "alert(1)", Confidence: 85}, // Duplicate
		{Value: "confirm(1)", Confidence: 90},
		{Value: "alert(1)", Confidence: 80}, // Duplicate
	}

	result := DeduplicatePayloads(payloads)

	if len(result) != 2 {
		t.Errorf("Expected 2 unique payloads, got %d", len(result))
	}

	// First occurrence should be kept
	if result[0].Confidence != 90 {
		t.Error("First occurrence should be kept (confidence 90)")
	}
}

func TestGenerateBulkVariations(t *testing.T) {
	payloads := []XSSPayload{
		{Value: `<img src=x onerror=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, Confidence: 90},
		{Value: `<svg onload=alert(1)>`, Categories: []XSSPayloadCategory{CategoryTagInjection}, Confidence: 85},
	}

	config := VariationConfig{
		EnableDialogFunction: true,
		MaxVariations:        10,
	}

	result := GenerateBulkVariations(payloads, config)

	// Should have more payloads after variation generation
	if len(result) <= len(payloads) {
		t.Errorf("Expected more payloads after bulk variation, got %d", len(result))
	}

	// Should be deduplicated
	seen := make(map[string]bool)
	for _, p := range result {
		if seen[p.Value] {
			t.Errorf("Found duplicate payload: %s", p.Value)
		}
		seen[p.Value] = true
	}
}

func TestDefaultVariationConfig(t *testing.T) {
	config := DefaultVariationConfig()

	// Check defaults
	if !config.EnableCaseMix {
		t.Error("EnableCaseMix should be true by default")
	}
	if !config.EnableDialogFunction {
		t.Error("EnableDialogFunction should be true by default")
	}
	if !config.EnableWhitespace {
		t.Error("EnableWhitespace should be true by default")
	}
	if config.EnableUnicodeJS {
		t.Error("EnableUnicodeJS should be false by default")
	}
	if config.MaxVariations != 15 {
		t.Errorf("MaxVariations should be 15 by default, got %d", config.MaxVariations)
	}
}
