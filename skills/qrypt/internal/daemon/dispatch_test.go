package daemon

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalParams(t *testing.T) {
	params := map[string]interface{}{
		"mount_point": "/test/path",
	}

	var target struct {
		MountPoint string `json:"mount_point"`
	}
	if err := unmarshalParams(params, &target); err != nil {
		t.Fatal(err)
	}
	if target.MountPoint != "/test/path" {
		t.Fatalf("got mount_point=%s", target.MountPoint)
	}
}

func TestUnmarshalParamsJSON(t *testing.T) {
	raw := `{"cookie":"test_cookie"}`
	var params interface{}
	json.Unmarshal([]byte(raw), &params)

	var target struct {
		Cookie string `json:"cookie"`
	}
	if err := unmarshalParams(params, &target); err != nil {
		t.Fatal(err)
	}
	if target.Cookie != "test_cookie" {
		t.Fatalf("got cookie=%s", target.Cookie)
	}
}
