package wsfuzz

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/PaesslerAG/jsonpath"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
)

// HTTPResponseRef is the read-back shape of a PreSetup HTTP response that
// extractions can pull from. Headers is the rendered map (project convention
// uses single-value-per-header for the playground PreSetup case).
type HTTPResponseRef struct {
	StatusCode int
	Headers    map[string]string
	Body       string
}

// ApplyExtraction runs one extraction against the provided sources and returns
// the captured string + an ok flag. ok=false means the extraction failed (no
// match, malformed pattern, invalid method for the source). Caller decides
// whether to abort or fall back to empty based on FallbackPolicy.
func ApplyExtraction(ext Extraction, frames []wsreplay.Frame, httpResp *HTTPResponseRef) (string, bool) {
	switch ext.Source {
	case SourceLastReceivedFrame:
		f, ok := lastReceivedFrame(frames)
		if !ok {
			return "", false
		}
		return extractFromFrame(ext, f)
	case SourceStepReceived:
		received := receivedFrames(frames)
		if ext.StepIndex < 0 || ext.StepIndex >= len(received) {
			return "", false
		}
		return extractFromFrame(ext, received[ext.StepIndex])
	case SourceHTTPResponse:
		if httpResp == nil {
			return "", false
		}
		return extractFromHTTPResponse(ext, *httpResp)
	}
	return "", false
}

func lastReceivedFrame(frames []wsreplay.Frame) (wsreplay.Frame, bool) {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].Direction == "received" {
			return frames[i], true
		}
	}
	return wsreplay.Frame{}, false
}

func receivedFrames(frames []wsreplay.Frame) []wsreplay.Frame {
	out := make([]wsreplay.Frame, 0, len(frames))
	for _, f := range frames {
		if f.Direction == "received" {
			out = append(out, f)
		}
	}
	return out
}

func extractFromFrame(ext Extraction, f wsreplay.Frame) (string, bool) {
	switch ext.Method {
	case MethodFull:
		if f.Opcode == 2 {
			return base64.StdEncoding.EncodeToString([]byte(f.Content)), true
		}
		return f.Content, true
	case MethodRegexGroup:
		return applyRegexGroup(ext, f.Content)
	case MethodJSONPath:
		if f.Opcode == 2 {
			return "", false
		}
		return applyJSONPath(ext, f.Content)
	case MethodHeader:
		return "", false // header method is http_response only
	}
	return "", false
}

func extractFromHTTPResponse(ext Extraction, r HTTPResponseRef) (string, bool) {
	switch ext.Method {
	case MethodFull:
		return r.Body, true
	case MethodRegexGroup:
		return applyRegexGroup(ext, r.Body)
	case MethodJSONPath:
		return applyJSONPath(ext, r.Body)
	case MethodHeader:
		if ext.HeaderName == "" {
			return "", false
		}
		for k, v := range r.Headers {
			if strings.EqualFold(k, ext.HeaderName) {
				return v, true
			}
		}
		return "", false
	}
	return "", false
}

func applyRegexGroup(ext Extraction, s string) (string, bool) {
	re, err := regexp.Compile(ext.Pattern)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	groupIdx := 1
	if ext.GroupOrPath != "" {
		if n, err := strconv.Atoi(ext.GroupOrPath); err == nil {
			groupIdx = n
		}
	}
	if groupIdx < 0 || groupIdx >= len(m) {
		return "", false
	}
	return m[groupIdx], true
}

func applyJSONPath(ext Extraction, s string) (string, bool) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", false
	}
	res, err := jsonpath.Get(ext.GroupOrPath, v)
	if err != nil {
		return "", false
	}
	switch t := res.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64), true
	case bool:
		return strconv.FormatBool(t), true
	case nil:
		return "", true
	}
	b, _ := json.Marshal(res)
	return string(b), true
}
