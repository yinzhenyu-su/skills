package protocol

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestEncodeDecodeRequest(t *testing.T) {
	req := &Request{ID: 1, Method: "status", Params: map[string]string{"k": "v"}}
	var buf bytes.Buffer
	if err := EncodeRequest(&buf, req); err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeRequest(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != 1 || decoded.Method != "status" {
		t.Fatalf("got id=%d method=%s", decoded.ID, decoded.Method)
	}
}

func TestEncodeDecodeResponse(t *testing.T) {
	resp := NewResult(1, map[string]string{"ok": "true"})
	var buf bytes.Buffer
	if err := EncodeResponse(&buf, resp); err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeResponse(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != 1 || decoded.Error != nil {
		t.Fatalf("got id=%d error=%v", decoded.ID, decoded.Error)
	}
}

func TestErrorResponse(t *testing.T) {
	resp := NewError(1, ErrCodeMethodNotFound, "not found")
	if resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("got code=%d", resp.Error.Code)
	}
	var buf bytes.Buffer
	if err := EncodeResponse(&buf, resp); err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeResponse(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Error == nil || decoded.Error.Code != ErrCodeMethodNotFound {
		t.Fatalf("got code=%d", decoded.Error.Code)
	}
}

func TestEncodeEvent(t *testing.T) {
	evt := &Event{
		Type:      EventMountStateChanged,
		Timestamp: 1000,
		Data:      map[string]string{"state": "mounted"},
	}

	var buf bytes.Buffer
	if err := EncodeEvent(&buf, evt); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, `"method":"event"`) {
		t.Fatalf("missing method \"event\" in output: %s", output)
	}
	if !strings.Contains(output, `"mount_state_changed"`) {
		t.Fatalf("missing event type in output: %s", output)
	}
	if !strings.Contains(output, `"mounted"`) {
		t.Fatalf("missing state data in output: %s", output)
	}
}

func TestEncodeDecodeEmptyBody(t *testing.T) {
	req := &Request{ID: 2, Method: "ping"}
	var buf bytes.Buffer
	if err := EncodeRequest(&buf, req); err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeRequest(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != 2 || decoded.Method != "ping" {
		t.Fatalf("got id=%d method=%s", decoded.ID, decoded.Method)
	}
}
