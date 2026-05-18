package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// EncodeRequest writes a JSON-RPC request as a single JSON line + newline
func EncodeRequest(w io.Writer, req *Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request failed: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// EncodeResponse writes a JSON-RPC response as a single JSON line + newline
func EncodeResponse(w io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response failed: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// EncodeEvent writes an event as a JSON-line Request with method "event"
func EncodeEvent(w io.Writer, evt *Event) error {
	return EncodeRequest(w, &Request{
		ID:     0,
		Method: "event",
		Params: evt,
	})
}

// DecodeRequest reads a single JSON-RPC request from a line-based reader
func DecodeRequest(r *bufio.Reader) (*Request, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = trim(line)
	if len(line) == 0 {
		return nil, nil
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request failed: %w", err)
	}
	return &req, nil
}

// DecodeResponse reads a single JSON-RPC response from a line-based reader
func DecodeResponse(r *bufio.Reader) (*Response, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = trim(line)
	if len(line) == 0 {
		return nil, nil
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w", err)
	}
	return &resp, nil
}

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
func NewResult(id int64, result interface{}) *Response {
	return &Response{
		ID:     id,
		Result: result,
	}
}

func trim(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}
