package active

import (
	"context"
	"net/http"
	"sync"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

// isForbiddenStatus reports whether a status code still represents an access
// denial (401/403), the class of baseline these audits require to remain true.
func isForbiddenStatus(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

// controlIsOpen distinguishes the two reasons a control can fail to reproduce the
// denial. A 2xx/3xx control proves the stored baseline is stale; a 429 or 5xx only
// means we could not confirm it — possibly because of the bypass probes that just
// ran. Both suppress the finding, but only the first is evidence of anything.
func controlIsOpen(code int) bool {
	return code >= 200 && code < 400
}

// replayOriginalRequest resends the stored history item exactly as-is: same
// method, URL, headers and body, without any injected bypass headers or path
// mutations.
func replayOriginalRequest(ctx context.Context, client *http.Client, original *db.History, options ActiveModuleOptions) (*db.History, error) {
	req, err := http_utils.BuildRequestFromHistoryItem(original)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:              db.SourceScanner,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
			ScanID:              options.ScanID,
			ScanJobID:           options.ScanJobID,
			CreateNewBodyStream: false,
		},
	})
	if result.Err != nil {
		return nil, result.Err
	}
	return result.History, nil
}

// baselineControl re-validates a stored blocked baseline against a fresh,
// unmodified replay of the original request. Crawled baselines go stale
// (e.g. the account they were captured against gets created moments later),
// so a bypass is only real if the control still gets denied. The replay is
// performed at most once per scan of a history item, no matter how many
// bypass combinations run concurrently against it.
type baselineControl struct {
	once    sync.Once
	history *db.History
	err     error
}

// resolve lazily fetches the control response, replaying it only the first
// time it's called for this baseline. Safe to call from multiple goroutines.
func (b *baselineControl) resolve(ctx context.Context, client *http.Client, original *db.History, options ActiveModuleOptions) (*db.History, error) {
	b.once.Do(func() {
		b.history, b.err = replayOriginalRequest(ctx, client, original, options)
	})
	return b.history, b.err
}
