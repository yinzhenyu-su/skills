package driver

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeOSSCallbackPreservesQuarkPayload(t *testing.T) {
	raw := json.RawMessage(`{"callbackUrl":"https://drive.quark.cn/callback","callbackBody":"bucket=${bucket}&object=${object}","callbackBodyType":"application/x-www-form-urlencoded"}`)
	encoded, err := encodeOSSCallback(raw)
	if err != nil {
		t.Fatalf("encodeOSSCallback failed: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode callback: %v", err)
	}

	if strings.TrimSpace(string(decoded)) != string(raw) {
		t.Fatalf("callback payload mismatch, got: %s", string(decoded))
	}
}