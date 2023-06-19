package db

type Task struct {
	BaseModel
	Status string `json:"status"`
}
