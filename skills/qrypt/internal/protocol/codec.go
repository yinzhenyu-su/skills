package protocol

// NewError creates a JSON-RPC error response
func NewError(id int64, code int, msg string) *Response {
	return &Response{
		ID: id,
		Error: &ErrorObj{
			Code:    code,
			Message: msg,
		},
	}
}

// NewResult creates a JSON-RPC success response
func NewResult(id int64, result any) *Response {
	return &Response{
		ID:     id,
		Result: result,
	}
}
