package wsfuzz

import (
	"testing"

	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

func mkFrame(opcode int, content string) wsreplay.Frame {
	return wsreplay.Frame{Opcode: opcode, Content: content, Direction: "received"}
}

func TestExtract_RegexGroupOnTextFrame(t *testing.T) {
	frames := []wsreplay.Frame{mkFrame(1, `{"token":"abc.def","ok":true}`)}
	val, ok := ApplyExtraction(Extraction{
		Source:      SourceLastReceivedFrame,
		Method:      MethodRegexGroup,
		Pattern:     `"token":"([^"]+)"`,
		GroupOrPath: "1",
	}, frames, nil)
	require.True(t, ok)
	require.Equal(t, "abc.def", val)
}

func TestExtract_JSONPathOnTextFrame(t *testing.T) {
	frames := []wsreplay.Frame{mkFrame(1, `{"data":{"id":"xyz"}}`)}
	val, ok := ApplyExtraction(Extraction{
		Source:      SourceLastReceivedFrame,
		Method:      MethodJSONPath,
		GroupOrPath: "$.data.id",
	}, frames, nil)
	require.True(t, ok)
	require.Equal(t, "xyz", val)
}

func TestExtract_FullOnTextFrame(t *testing.T) {
	frames := []wsreplay.Frame{mkFrame(1, `hello world`)}
	val, ok := ApplyExtraction(Extraction{
		Source: SourceLastReceivedFrame,
		Method: MethodFull,
	}, frames, nil)
	require.True(t, ok)
	require.Equal(t, "hello world", val)
}

func TestExtract_FullOnBinaryFrame_Base64(t *testing.T) {
	frames := []wsreplay.Frame{mkFrame(2, string([]byte{0x01, 0x02, 0x03}))}
	val, ok := ApplyExtraction(Extraction{
		Source: SourceLastReceivedFrame,
		Method: MethodFull,
	}, frames, nil)
	require.True(t, ok)
	require.Equal(t, "AQID", val) // base64 of 0x01 0x02 0x03
}

func TestExtract_JSONPathRejectedOnBinary(t *testing.T) {
	frames := []wsreplay.Frame{mkFrame(2, string([]byte{0xff, 0xfe}))}
	_, ok := ApplyExtraction(Extraction{
		Source:      SourceLastReceivedFrame,
		Method:      MethodJSONPath,
		GroupOrPath: "$.x",
	}, frames, nil)
	require.False(t, ok, "json_path must be invalid for binary frames")
}

func TestExtract_StepReceivedIndexing(t *testing.T) {
	frames := []wsreplay.Frame{
		mkFrame(1, "first"),
		mkFrame(1, "second"),
		mkFrame(1, "third"),
	}
	val, ok := ApplyExtraction(Extraction{
		Source:    SourceStepReceived,
		StepIndex: 1, // index into received frames
		Method:    MethodFull,
	}, frames, nil)
	require.True(t, ok)
	require.Equal(t, "second", val)
}

func TestExtract_PatternNoMatchReturnsFalse(t *testing.T) {
	frames := []wsreplay.Frame{mkFrame(1, "no token here")}
	_, ok := ApplyExtraction(Extraction{
		Source:      SourceLastReceivedFrame,
		Method:      MethodRegexGroup,
		Pattern:     `"token":"([^"]+)"`,
		GroupOrPath: "1",
	}, frames, nil)
	require.False(t, ok)
}

func TestExtract_HTTPResponseHeader(t *testing.T) {
	r := &HTTPResponseRef{
		StatusCode: 200,
		Headers:    map[string]string{"Set-Cookie": "sess=abc", "Content-Type": "application/json"},
		Body:       `{"ok":true}`,
	}
	val, ok := ApplyExtraction(Extraction{
		Source:     SourceHTTPResponse,
		Method:     MethodHeader,
		HeaderName: "Set-Cookie",
	}, nil, r)
	require.True(t, ok)
	require.Equal(t, "sess=abc", val)
}

func TestExtract_HTTPResponseHeaderCaseInsensitive(t *testing.T) {
	r := &HTTPResponseRef{
		Headers: map[string]string{"Set-Cookie": "x=y"},
	}
	val, ok := ApplyExtraction(Extraction{
		Source:     SourceHTTPResponse,
		Method:     MethodHeader,
		HeaderName: "set-cookie",
	}, nil, r)
	require.True(t, ok)
	require.Equal(t, "x=y", val)
}

func TestExtract_HTTPResponseJSONPath(t *testing.T) {
	r := &HTTPResponseRef{Body: `{"token":"abc.def"}`}
	val, ok := ApplyExtraction(Extraction{
		Source:      SourceHTTPResponse,
		Method:      MethodJSONPath,
		GroupOrPath: "$.token",
	}, nil, r)
	require.True(t, ok)
	require.Equal(t, "abc.def", val)
}
