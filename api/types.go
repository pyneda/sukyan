package api

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type ActionResponse struct {
	Message string `json:"message"`
}
