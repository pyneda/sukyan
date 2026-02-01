package core

import (
	"context"
	"net/http"
	"sync"

	"github.com/pyneda/sukyan/db"
)

type TestCategory string

const (
	TestCategoryShared   TestCategory = "shared"
	TestCategorySchema   TestCategory = "schema"
	TestCategoryOpenAPI  TestCategory = "openapi"
	TestCategoryGraphQL  TestCategory = "graphql"
	TestCategorySOAP     TestCategory = "soap"
)

type TestResult struct {
	Vulnerable  bool         `json:"vulnerable"`
	IssueCode   db.IssueCode `json:"issue_code"`
	Details     string       `json:"details"`
	Confidence  int          `json:"confidence"`
	Evidence    []byte       `json:"evidence,omitempty"`
	History     *db.History  `json:"history,omitempty"`
	Parameter   *Parameter   `json:"parameter,omitempty"`
	PayloadUsed string       `json:"payload_used,omitempty"`
}

type TestContext struct {
	Ctx         context.Context
	WorkspaceID uint
	TaskID      uint
	ScanID      uint
	ScanJobID   uint
	HTTPClient  *http.Client
	Definition  *db.APIDefinition
	Endpoint    *db.APIEndpoint
	Operation   *Operation
	BaseHistory *db.History
}

type APITest interface {
	Name() string
	Category() TestCategory
	ApplicableTo() []APIType
	Run(ctx TestContext) []TestResult
}

type TestRegistry struct {
	mu    sync.RWMutex
	tests map[TestCategory][]APITest
}

var globalRegistry = &TestRegistry{
	tests: make(map[TestCategory][]APITest),
}

func GetRegistry() *TestRegistry {
	return globalRegistry
}

func (r *TestRegistry) Register(test APITest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tests[test.Category()] = append(r.tests[test.Category()], test)
}

func (r *TestRegistry) GetTests(category TestCategory) []APITest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tests[category]
}

func (r *TestRegistry) GetTestsForAPIType(apiType APIType) []APITest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []APITest
	for _, tests := range r.tests {
		for _, test := range tests {
			for _, applicable := range test.ApplicableTo() {
				if applicable == apiType {
					result = append(result, test)
					break
				}
			}
		}
	}
	return result
}

func (r *TestRegistry) GetAllTests() []APITest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []APITest
	for _, tests := range r.tests {
		result = append(result, tests...)
	}
	return result
}

func (r *TestRegistry) GetSharedTests() []APITest {
	return r.GetTests(TestCategoryShared)
}

func (r *TestRegistry) GetSchemaTests() []APITest {
	return r.GetTests(TestCategorySchema)
}

func (r *TestRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, tests := range r.tests {
		count += len(tests)
	}
	return count
}

func RegisterTest(test APITest) {
	globalRegistry.Register(test)
}

type BaseTest struct {
	TestName     string
	TestCategory TestCategory
	Applicable   []APIType
}

func (t *BaseTest) Name() string {
	return t.TestName
}

func (t *BaseTest) Category() TestCategory {
	return t.TestCategory
}

func (t *BaseTest) ApplicableTo() []APIType {
	if len(t.Applicable) == 0 {
		return []APIType{APITypeOpenAPI, APITypeGraphQL, APITypeSOAP}
	}
	return t.Applicable
}
