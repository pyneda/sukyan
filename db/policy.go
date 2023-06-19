package db

type Policy struct {
	BaseModel
	Name        string `json:"name"`
	Description string `json:"description"`
}
