package core

import (
	"fmt"
)

type Parameter struct {
	Name          string            `json:"name"`
	Location      ParameterLocation `json:"location"`
	Required      bool              `json:"required"`
	DataType      DataType          `json:"data_type"`
	Constraints   Constraints       `json:"constraints"`
	DefaultValue  any               `json:"default_value,omitempty"`
	ExampleValue  any               `json:"example_value,omitempty"`
	NestedParams  []Parameter       `json:"nested_params,omitempty"`
	Description   string            `json:"description,omitempty"`
	Deprecated    bool              `json:"deprecated,omitempty"`
	AllowEmpty    bool              `json:"allow_empty,omitempty"`
	Nullable      bool              `json:"nullable,omitempty"`
	Style         string            `json:"style,omitempty"`
	Explode       *bool             `json:"explode,omitempty"`
	ContentType   string            `json:"content_type,omitempty"`
	SchemaRef     string            `json:"schema_ref,omitempty"`
}

func (p Parameter) String() string {
	required := ""
	if p.Required {
		required = " (required)"
	}
	return fmt.Sprintf("%s [%s] %s%s", p.Name, p.Location, p.DataType, required)
}

func (p Parameter) HasConstraints() bool {
	return !p.Constraints.IsEmpty()
}

const maxNestedDepth = 10

func (p Parameter) GetEffectiveValue() any {
	return p.getEffectiveValueWithDepth(0)
}

func (p Parameter) getEffectiveValueWithDepth(depth int) any {
	if p.ExampleValue != nil {
		return p.ExampleValue
	}
	if p.DefaultValue != nil {
		return p.DefaultValue
	}
	return p.generateDefaultValueWithDepth(depth)
}

func (p Parameter) GenerateDefaultValue() any {
	return p.generateDefaultValueWithDepth(0)
}

func (p Parameter) generateDefaultValueWithDepth(depth int) any {
	if depth > maxNestedDepth {
		return nil
	}

	switch p.DataType {
	case DataTypeString:
		if len(p.Constraints.Enum) > 0 {
			return p.Constraints.Enum[0]
		}
		if p.Constraints.Format == "email" {
			return "test@example.com"
		}
		if p.Constraints.Format == "uri" || p.Constraints.Format == "url" {
			return "https://example.com"
		}
		if p.Constraints.Format == "uuid" {
			return "00000000-0000-0000-0000-000000000001"
		}
		if p.Constraints.Format == "date" {
			return "2024-01-01"
		}
		if p.Constraints.Format == "date-time" {
			return "2024-01-01T00:00:00Z"
		}
		if p.Constraints.Format == "ipv4" {
			return "127.0.0.1"
		}
		if p.Constraints.Format == "ipv6" {
			return "::1"
		}
		if p.Constraints.MinLength != nil && *p.Constraints.MinLength > 0 {
			result := ""
			for i := 0; i < *p.Constraints.MinLength; i++ {
				result += "a"
			}
			return result
		}
		return "test"
	case DataTypeInteger:
		if len(p.Constraints.Enum) > 0 {
			return p.Constraints.Enum[0]
		}
		if p.Constraints.Minimum != nil {
			return int(*p.Constraints.Minimum)
		}
		return 1
	case DataTypeNumber:
		if len(p.Constraints.Enum) > 0 {
			return p.Constraints.Enum[0]
		}
		if p.Constraints.Minimum != nil {
			return *p.Constraints.Minimum
		}
		return 1.0
	case DataTypeBoolean:
		return true
	case DataTypeArray:
		if len(p.NestedParams) > 0 {
			return []any{p.NestedParams[0].getEffectiveValueWithDepth(depth + 1)}
		}
		return []any{}
	case DataTypeObject:
		obj := make(map[string]any)
		for _, nested := range p.NestedParams {
			obj[nested.Name] = nested.getEffectiveValueWithDepth(depth + 1)
		}
		return obj
	default:
		return nil
	}
}

func (p Parameter) IsPathParam() bool {
	return p.Location == ParameterLocationPath
}

func (p Parameter) IsQueryParam() bool {
	return p.Location == ParameterLocationQuery
}

func (p Parameter) IsHeaderParam() bool {
	return p.Location == ParameterLocationHeader
}

func (p Parameter) IsCookieParam() bool {
	return p.Location == ParameterLocationCookie
}

func (p Parameter) IsBodyParam() bool {
	return p.Location == ParameterLocationBody
}

type ParameterSet struct {
	Parameters []Parameter
}

func NewParameterSet(params ...Parameter) *ParameterSet {
	return &ParameterSet{Parameters: params}
}

func (ps *ParameterSet) Add(param Parameter) {
	ps.Parameters = append(ps.Parameters, param)
}

func (ps *ParameterSet) GetByName(name string) *Parameter {
	for i := range ps.Parameters {
		if ps.Parameters[i].Name == name {
			return &ps.Parameters[i]
		}
	}
	return nil
}

func (ps *ParameterSet) GetByLocation(location ParameterLocation) []Parameter {
	var result []Parameter
	for _, p := range ps.Parameters {
		if p.Location == location {
			result = append(result, p)
		}
	}
	return result
}

func (ps *ParameterSet) GetRequired() []Parameter {
	var result []Parameter
	for _, p := range ps.Parameters {
		if p.Required {
			result = append(result, p)
		}
	}
	return result
}

func (ps *ParameterSet) GetPathParams() []Parameter {
	return ps.GetByLocation(ParameterLocationPath)
}

func (ps *ParameterSet) GetQueryParams() []Parameter {
	return ps.GetByLocation(ParameterLocationQuery)
}

func (ps *ParameterSet) GetHeaderParams() []Parameter {
	return ps.GetByLocation(ParameterLocationHeader)
}

func (ps *ParameterSet) GetCookieParams() []Parameter {
	return ps.GetByLocation(ParameterLocationCookie)
}

func (ps *ParameterSet) GetBodyParams() []Parameter {
	return ps.GetByLocation(ParameterLocationBody)
}
