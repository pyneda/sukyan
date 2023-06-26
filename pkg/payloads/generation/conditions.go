package generation

type DetectionCondition string

var (
	Or  DetectionCondition = "or"
	And DetectionCondition = "and"
)

type ResponseConditionCheck string

var (
	DatabaseErrorCondition ResponseConditionCheck = "database_error"
)
