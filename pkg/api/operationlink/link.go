// Package operationlink joins parsed operations to the endpoint rows they belong
// to.
//
// It is its own package rather than part of pkg/api because pkg/api imports
// pkg/discovery, and pkg/discovery is one of the two callers.
package operationlink

import (
	"encoding/json"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
)

// AttachOperationJSON serializes each parsed operation onto the endpoint row it
// belongs to and reports how many rows were matched.
//
// The three importers in pkg/discovery build rows differently — OpenAPI keys on
// path and method, GraphQL and SOAP leave the path empty and key on the operation
// name — so the join key is protocol-specific. An unmatched row keeps a nil
// OperationJSON and the detail API reports it honestly rather than inventing one.
func AttachOperationJSON(endpoints []*db.APIEndpoint, ops []core.Operation, defType db.APIDefinitionType) int {
	if len(endpoints) == 0 || len(ops) == 0 {
		return 0
	}

	byKey := make(map[string]*core.Operation, len(ops))
	for i := range ops {
		byKey[operationKey(&ops[i], defType)] = &ops[i]
	}

	matched := 0
	for _, endpoint := range endpoints {
		op, ok := byKey[endpointKey(endpoint, defType)]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(op)
		if err != nil {
			continue
		}
		endpoint.OperationJSON = encoded
		matched++
	}
	return matched
}

func operationKey(op *core.Operation, defType db.APIDefinitionType) string {
	switch defType {
	case db.APIDefinitionTypeGraphQL:
		operationType := ""
		if op.GraphQL != nil {
			operationType = op.GraphQL.OperationType
		}
		return strings.ToLower(operationName(op)) + "\x00" + strings.ToLower(operationType)
	case db.APIDefinitionTypeWSDL:
		action := ""
		if op.SOAP != nil {
			action = op.SOAP.SOAPAction
		}
		return strings.ToLower(operationName(op)) + "\x00" + action
	default:
		return op.Path + "\x00" + strings.ToUpper(op.Method)
	}
}

func endpointKey(endpoint *db.APIEndpoint, defType db.APIDefinitionType) string {
	switch defType {
	case db.APIDefinitionTypeGraphQL:
		return strings.ToLower(endpointName(endpoint)) + "\x00" + strings.ToLower(endpoint.OperationType)
	case db.APIDefinitionTypeWSDL:
		return strings.ToLower(endpointName(endpoint)) + "\x00" + endpoint.SOAPAction
	default:
		return endpoint.Path + "\x00" + strings.ToUpper(endpoint.Method)
	}
}

func operationName(op *core.Operation) string {
	if op.OperationID != "" {
		return op.OperationID
	}
	return op.Name
}

func endpointName(endpoint *db.APIEndpoint) string {
	if endpoint.OperationID != "" {
		return endpoint.OperationID
	}
	return endpoint.Name
}
