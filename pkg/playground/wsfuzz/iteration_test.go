package wsfuzz

import (
	"context"
	"testing"
	"time"

	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

type fakeSession struct {
	sentFrames []wsreplay.Frame
	frameQueue []wsreplay.Frame
	closed     bool
}

func (f *fakeSession) Send(opcode int, content string) error {
	f.sentFrames = append(f.sentFrames, wsreplay.Frame{Opcode: opcode, Content: content, Direction: "sent"})
	return nil
}
func (f *fakeSession) NextFrame(timeout time.Duration) (wsreplay.Frame, error) {
	if len(f.frameQueue) == 0 {
		time.Sleep(timeout)
		return wsreplay.Frame{}, &fakeTimeoutErr{}
	}
	out := f.frameQueue[0]
	f.frameQueue = f.frameQueue[1:]
	return out, nil
}
func (f *fakeSession) Close()             { f.closed = true }
func (f *fakeSession) ConnectionID() uint { return 42 }

type fakeTimeoutErr struct{}

func (*fakeTimeoutErr) Error() string { return "timeout" }

func TestRunIteration_BasicTwoStep(t *testing.T) {
	fake := &fakeSession{
		frameQueue: []wsreplay.Frame{
			{Opcode: 1, Content: "auth_ok", Direction: "received"},
			{Opcode: 1, Content: "msg_ok", Direction: "received"},
		},
	}
	cfg := WsFuzzerConfig{
		TargetURL: "ws://test/ws",
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{
				Role: RoleSetup, Opcode: 1, Content: `{"type":"auth"}`,
				WaitFor: &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "auth_ok", TimeoutMs: 1000},
			},
			{
				Role:      RoleFuzz,
				Opcode:    1,
				Content:   `{"type":"msg","text":"X"}`,
				Positions: []fuzz.FuzzerPosition{{Start: 23, End: 24, OriginalValue: "X"}},
				WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "msg_ok", TimeoutMs: 1000},
			},
		},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"hello"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{RequestTimeoutSeconds: 5},
	}
	refs := BuildPositionRefs(cfg.Script)
	deps := IterationDeps{
		Dial: func(ctx context.Context, _ wsreplay.SessionConfig) (SessionHandle, error) { return fake, nil },
	}
	res, err := RunIteration(context.Background(), cfg, 0, refs, []string{"hello"}, nil, nil, deps)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, res.Status)
	require.Len(t, fake.sentFrames, 2)
	require.Contains(t, fake.sentFrames[1].Content, "hello")
}
