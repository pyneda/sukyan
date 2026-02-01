package api

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/active"
	apicore "github.com/pyneda/sukyan/pkg/api/core"
)

type APITestOptions struct {
	active.ActiveModuleOptions

	Definition  *db.APIDefinition
	Endpoint    *db.APIEndpoint
	BaseHistory *db.History
	Operation   *apicore.Operation
}

type APITestResult struct {
	Vulnerable bool
	IssueCode  db.IssueCode
	Details    string
	Confidence int
	Evidence   []byte
	History    *db.History
}
