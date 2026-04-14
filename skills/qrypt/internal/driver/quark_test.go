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

func TestFileUnmarshaling(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected int64
	}{
		{
			name: "size as number",
			jsonData: `{
				"fid": "test_fid",
				"file_name": "test_file.txt",
				"size": 12345,
				"file": true
			}`,
			expected: 12345,
		},
		{
			name: "size as string",
			jsonData: `{
				"fid": "test_fid",
				"file_name": "test_file.txt",
				"size": "67890",
				"file": true
			}`,
			expected: 67890,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f File
			err := json.Unmarshal([]byte(tt.jsonData), &f)
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if f.Int64Size() != tt.expected {
				t.Errorf("Expected Size %d, got %d", tt.expected, f.Int64Size())
			}
		})
	}
}
