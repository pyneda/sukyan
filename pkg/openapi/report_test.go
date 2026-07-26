package openapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateReportJSON(t *testing.T) {
	endpoints := []Endpoint{{
		Method:   "GET",
		Path:     "/items",
		Requests: []RequestVariation{{Label: "Happy Path", URL: "http://target/items"}},
	}}

	var buffer bytes.Buffer
	if err := GenerateReport(endpoints, ReportFormatJSON, &buffer); err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	var decoded []Endpoint
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Path != "/items" {
		t.Errorf("decoded report = %+v, want the original endpoint", decoded)
	}
}

func TestGenerateReportUnsupportedFormat(t *testing.T) {
	var buffer bytes.Buffer
	if err := GenerateReport(nil, ReportFormat("pdf"), &buffer); err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
}

// TestGenerateReportHTMLEscapesSpecContent guards against a spec breaking out of the
// report's script block. Every string here comes from a scan target.
func TestGenerateReportHTMLEscapesSpecContent(t *testing.T) {
	endpoints := []Endpoint{{
		Method:      "GET",
		Path:        `/items</script><script>alert(1)</script>`,
		Summary:     `</script><img src=x onerror=alert(2)>`,
		Description: `"><svg onload=alert(3)>`,
		Requests: []RequestVariation{{
			Label: `</script>alert(4)`,
			URL:   `http://target/items?q=</script>`,
		}},
	}}

	var buffer bytes.Buffer
	if err := GenerateReport(endpoints, ReportFormatHTML, &buffer); err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	rendered := buffer.String()
	if strings.Contains(rendered, "</script><script>alert(1)</script>") {
		t.Error("spec-controlled path escaped the script block")
	}
	if strings.Contains(rendered, "<img src=x onerror=alert(2)>") {
		t.Error("spec-controlled summary escaped the script block")
	}
	if strings.Contains(rendered, "<svg onload=alert(3)>") {
		t.Error("spec-controlled description escaped the script block")
	}
}

func TestGenerateReportHTMLRendersEndpoints(t *testing.T) {
	doc := mustParse(t, `{"openapi":"3.0.0","info":{"title":"x","version":"1"},
	  "paths":{"/items":{"get":{"summary":"List items","responses":{"200":{"description":"ok"}}}}}}`)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	var buffer bytes.Buffer
	if err := GenerateReport(endpoints, ReportFormatHTML, &buffer); err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if !strings.Contains(buffer.String(), "List items") {
		t.Error("rendered report does not contain the endpoint summary")
	}
}
