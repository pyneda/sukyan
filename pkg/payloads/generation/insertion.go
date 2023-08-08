package generation

type InsertionMode string

const (
	Append  InsertionMode = "append"
	Prepend InsertionMode = "prepend"
	Replace InsertionMode = "replace"
)
