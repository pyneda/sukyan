package active

import (
	"context"
	"net/http"

	"github.com/pyneda/sukyan/pkg/scan/options"
)

type ActiveModuleOptions struct {
	Ctx         context.Context
	WorkspaceID uint
	TaskID      uint
	TaskJobID   uint
	ScanID      uint
	ScanJobID   uint
	Concurrency int
	ScanMode    options.ScanMode
	HTTPClient  *http.Client
	APIContext  *options.APIContext
}
