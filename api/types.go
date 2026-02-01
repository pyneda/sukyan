package api

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func NewErrorResponse(err string, details ...string) ErrorResponse {
	resp := ErrorResponse{Error: err}
	if len(details) > 0 {
		resp.Message = details[0]
	}
	return resp
}

type ActionResponse struct {
	Message string `json:"message"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}
