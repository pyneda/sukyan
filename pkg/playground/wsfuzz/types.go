package wsfuzz

import (
	"crypto/tls"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
)

// StepRole partitions script steps by purpose.
type StepRole string

const (
	RoleSetup StepRole = "setup" // sent verbatim every iteration; runs once if in PreSetup
	RoleFuzz  StepRole = "fuzz"  // carries insertion points; payloads vary per iteration
	RoleCheck StepRole = "check" // assertion-only; failure marks iteration as a finding
)

// ContentKind hints the editor; the engine substitutes bytes regardless.
type ContentKind string

const (
	KindText   ContentKind = "text"
	KindJSON   ContentKind = "json"
	KindBinary ContentKind = "binary" // base64-encoded transport
	KindCustom ContentKind = "custom"
)

// PolicyAction is what to do when a wait_for misbehaves.
type PolicyAction string

const (
	PolicyAbort    PolicyAction = "abort"
	PolicyContinue PolicyAction = "continue"
)

// VarScope describes when a variable is valid.
type VarScope string

const (
	VarScopeRun       VarScope = "run"       // set once by PreSetup; constant across iterations
	VarScopeIteration VarScope = "iteration" // set mid-script; visible only within the iteration
)

// VariableSpec declares a variable name + its scope. Extractions populate them.
type VariableSpec struct {
	Name  string   `json:"name"`
	Scope VarScope `json:"scope"`
}

// ExtractSource names where an extraction reads from.
type ExtractSource string

const (
	SourceLastReceivedFrame ExtractSource = "last_received_frame"
	SourceStepReceived      ExtractSource = "step_received" // requires StepIndex
	SourceHTTPResponse      ExtractSource = "http_response" // PreSetup only
)

// ExtractMethod names how an extraction parses its source.
type ExtractMethod string

const (
	MethodRegexGroup ExtractMethod = "regex_group"
	MethodJSONPath   ExtractMethod = "json_path"
	MethodHeader     ExtractMethod = "header" // http_response only
	MethodFull       ExtractMethod = "full"
)

// FallbackAction is what to do when extraction fails.
type FallbackAction string

const (
	FallbackAbort         FallbackAction = "abort_iteration"
	FallbackContinueEmpty FallbackAction = "continue_with_empty"
)

// Extraction captures a value from a frame/response into a variable.
type Extraction struct {
	Name           string         `json:"name"`
	Source         ExtractSource  `json:"source"`
	StepIndex      int            `json:"step_index,omitempty"` // for Source=step_received
	Method         ExtractMethod  `json:"method"`
	Pattern        string         `json:"pattern,omitempty"`
	GroupOrPath    string         `json:"group_or_path,omitempty"`
	HeaderName     string         `json:"header_name,omitempty"`
	FallbackPolicy FallbackAction `json:"fallback_policy"`
}

// AssertionLogic combines a CheckAssertion's rules.
type AssertionLogic string

const (
	LogicAnd AssertionLogic = "and"
	LogicOr  AssertionLogic = "or"
)

// CheckAssertion is the success criterion of a check-role step.
type CheckAssertion struct {
	Logic  AssertionLogic     `json:"logic"`
	Negate bool               `json:"negate,omitempty"`
	Rules  []fuzz.MatcherRule `json:"rules"`
}

// WsFuzzStep is one step in the iteration script.
type WsFuzzStep struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Role        StepRole              `json:"role"`
	Opcode      int                   `json:"opcode"`       // 1 text, 2 binary
	ContentKind ContentKind           `json:"content_kind"` // editor hint
	Content     string                `json:"content"`      // may contain §§ markers and ${vars}
	DelayMs     int                   `json:"delay_ms"`
	Positions   []fuzz.FuzzerPosition `json:"positions,omitempty"` // role=fuzz only
	WaitFor     *wsreplay.WaitForSpec `json:"wait_for,omitempty"`
	Extract     []Extraction          `json:"extract,omitempty"`
	CheckAssert *CheckAssertion       `json:"check_assert,omitempty"` // role=check only
	OnTimeout   PolicyAction          `json:"on_timeout,omitempty"`
	OnNoMatch   PolicyAction          `json:"on_no_match,omitempty"`
}

// SetupKind picks the pre-iteration setup mechanism.
type SetupKind string

const (
	SetupNone        SetupKind = "none"
	SetupWsScript    SetupKind = "ws_script"
	SetupHTTPRequest SetupKind = "http_request"
)

// HTTPRequestSpec is a raw HTTP request to fire during PreSetup. Reused via
// the existing HTTP playground primitives. Holding only the minimal shape here.
type HTTPRequestSpec struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// PreSetup runs ONCE per run (not per iteration), producing run-scoped variables.
type PreSetup struct {
	Kind        SetupKind        `json:"kind"`
	Steps       []WsFuzzStep     `json:"steps,omitempty"`        // Kind=ws_script
	HTTPRequest *HTTPRequestSpec `json:"http_request,omitempty"` // Kind=http_request
	Extract     []Extraction     `json:"extract,omitempty"`
}

// TLSConfig describes the user-facing TLS knobs. Translated to *tls.Config
// at dial time (via BuildTLSConfig).
type TLSConfig struct {
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"`
	ServerName         string `json:"server_name,omitempty"`
	ClientCertPEM      string `json:"client_cert_pem,omitempty"`
	ClientKeyPEM       string `json:"client_key_pem,omitempty"`
}

// BuildTLSConfig produces *tls.Config; returns nil if the TLSConfig is zero-valued.
func BuildTLSConfig(c TLSConfig) *tls.Config {
	if c == (TLSConfig{}) {
		return nil
	}
	out := &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}
	if c.ClientCertPEM != "" && c.ClientKeyPEM != "" {
		cert, err := tls.X509KeyPair([]byte(c.ClientCertPEM), []byte(c.ClientKeyPEM))
		if err == nil {
			out.Certificates = []tls.Certificate{cert}
		}
	}
	return out
}

// WsFuzzerConfig is the persisted-on-session and snapshotted-on-run config.
type WsFuzzerConfig struct {
	TargetURL         string                `json:"target_url"`
	RequestHeaders    []wsreplay.HeaderSpec `json:"request_headers,omitempty"`
	ConnectionTimeout int                   `json:"connection_timeout_ms,omitempty"`
	Subprotocols      []string              `json:"subprotocols,omitempty"`
	TLSConfig         TLSConfig             `json:"tls_config,omitempty"`

	PreIterationSetup *PreSetup      `json:"pre_iteration_setup,omitempty"`
	Script            []WsFuzzStep   `json:"script"`
	Variables         []VariableSpec `json:"variables,omitempty"`

	Mode           fuzz.FuzzMode             `json:"mode"`
	SharedPayloads *fuzz.FuzzerPayloadsGroup `json:"shared_payloads,omitempty"`

	Matchers         fuzz.MatcherSet             `json:"matchers"`
	ExecutionOptions fuzz.FuzzerExecutionOptions `json:"execution_options"`
	AutoBaseline     fuzz.AutoBaselineMode       `json:"auto_baseline,omitempty"`
}

// IterationStatus enumerates the per-iteration terminal states.
type IterationStatus string

const (
	StatusCompleted            IterationStatus = "completed"
	StatusCheckFailed          IterationStatus = "check_failed"
	StatusStepFailedTimeout    IterationStatus = "step_failed_timeout"
	StatusStepFailedNoMatch    IterationStatus = "step_failed_no_match"
	StatusStepFailedExtraction IterationStatus = "step_failed_extraction"
	StatusPeerClosed           IterationStatus = "peer_closed"
	StatusConnectionError      IterationStatus = "connection_error"
	StatusIterationTimeout     IterationStatus = "iteration_timeout"
)

// CountsTowardErrorRate reports whether this terminal status increments the
// StopOnErrorRate watchdog.
func (s IterationStatus) CountsTowardErrorRate() bool {
	switch s {
	case StatusConnectionError, StatusIterationTimeout, StatusStepFailedTimeout:
		return true
	}
	return false
}

// FrameSig is one frame's contribution to the baseline fingerprint.
type FrameSig struct {
	Opcode      int    `json:"opcode"`
	SizeBytes   int    `json:"size_bytes"`
	ContentHash string `json:"content_hash"` // "sha256:<hex>" — 71 chars total
}

// WsBaselineFingerprint is the sequence-aware baseline shape.
type WsBaselineFingerprint struct {
	FrameCount      int        `json:"frame_count"`
	PerFrame        []FrameSig `json:"per_frame"`
	HandshakeStatus int        `json:"handshake_status"`
}

// WsPositionRef wraps a FuzzerPosition with its owning step index. The
// orchestrator builds these from the script; the mode strategies only see
// the flat []FuzzerPosition slice extracted from them.
type WsPositionRef struct {
	StepIndex int
	Position  fuzz.FuzzerPosition
}

// WsIterationResult is the projection published on the live stream and
// persisted in playground_ws_fuzz_iterations.
type WsIterationResult struct {
	RunID                 uint              `json:"run_id"`
	IterationIndex        int               `json:"iteration_index"`
	Status                IterationStatus   `json:"status"`
	PayloadValues         []string          `json:"payload_values"` // may be truncated on stream
	BaselineMatch         bool              `json:"baseline_match"`
	DurationMs            int               `json:"duration_ms"`
	HandshakeStatusCode   int               `json:"handshake_status_code"`
	WebSocketConnectionID *uint             `json:"websocket_connection_id,omitempty"`
	PeerCloseCode         *int              `json:"peer_close_code,omitempty"`
	FailureReason         string            `json:"failure_reason,omitempty"`
	FailedStepIndex       *int              `json:"failed_step_index,omitempty"`
	CheckResults          []byte            `json:"check_results,omitempty"` // raw JSON
	VariablesSnapshot     map[string]string `json:"variables_snapshot,omitempty"`
	Ts                    time.Time         `json:"ts"`
}
