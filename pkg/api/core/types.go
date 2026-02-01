package core

type APIType string

const (
	APITypeOpenAPI APIType = "openapi"
	APITypeGraphQL APIType = "graphql"
	APITypeSOAP    APIType = "soap"
)

type ParameterLocation string

const (
	ParameterLocationPath     ParameterLocation = "path"
	ParameterLocationQuery    ParameterLocation = "query"
	ParameterLocationHeader   ParameterLocation = "header"
	ParameterLocationCookie   ParameterLocation = "cookie"
	ParameterLocationBody     ParameterLocation = "body"
	ParameterLocationArgument ParameterLocation = "argument" // GraphQL
)

type DataType string

const (
	DataTypeString  DataType = "string"
	DataTypeInteger DataType = "integer"
	DataTypeNumber  DataType = "number"
	DataTypeBoolean DataType = "boolean"
	DataTypeArray   DataType = "array"
	DataTypeObject  DataType = "object"
	DataTypeFile    DataType = "file"
)

func (dt DataType) IsNumeric() bool {
	return dt == DataTypeInteger || dt == DataTypeNumber
}

func (dt DataType) IsString() bool {
	return dt == DataTypeString
}
