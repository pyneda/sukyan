package generation

type Operator string

var (
	Or  Operator = "or"
	And Operator = "and"
)

type ResponseConditionCheck string

var (
	DatabaseErrorCondition ResponseConditionCheck = "database_error"
	XPathErrorCondition    ResponseConditionCheck = "xpath_error"
)
