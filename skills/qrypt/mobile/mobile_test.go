package mobile

import (
	"strings"
	"testing"
)

func TestNewQryptAPI_RequiresConfig(t *testing.T) {
	if _, err := NewQryptAPI(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewQryptAPI_RequiresPasswordAndSalt(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
	}{
		{"empty", &Config{}},
		{"no salt", &Config{Password: "x"}},
		{"no password", &Config{Salt: "y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewQryptAPI(c.cfg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewQryptAPI_Defaults(t *testing.T) {
	api, err := NewQryptAPI(&Config{
		Password:  "test-pass",
		Salt:      "test-salt",
		CacheDir:  "/tmp/qrypt-cache",
		DataDir:   "/tmp/qrypt-data",
		ConfigDir: "/tmp/qrypt-config",
	})
	if err != nil {
		t.Fatalf("NewQryptAPI: %v", err)
	}
	defer api.Shutdown()

	if api.fileAPI == nil {
		t.Fatal("Inner() is nil")
	}
	if api.creds == nil {
		t.Fatal("creds not set")
	}
	if api.streams == nil {
		t.Fatal("streams not set")
	}
}

func TestSetCredential_InMemory(t *testing.T) {
	api, err := NewQryptAPI(&Config{
		Password: "p",
		Salt:     "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer api.Shutdown()

	if err := api.SetCredential("cookie_test", "value"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	if err := api.DeleteCredential("cookie_test"); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
}

func TestSetCredential_ProviderRejected(t *testing.T) {
	api, err := NewQryptAPIWithProvider(&Config{
		Password: "p",
		Salt:     "s",
	}, fakeProvider{})
	if err != nil {
		t.Fatal(err)
	}
	defer api.Shutdown()

	err = api.SetCredential("k", "v")
	if err == nil || !strings.Contains(err.Error(), "not in-memory") {
		t.Fatalf("expected not in-memory error, got %v", err)
	}
}

func TestSplitTypeMount(t *testing.T) {
	cases := []struct {
		in       string
		wantType string
		wantName string
	}{
		{"quark", "quark", "quark"},
		{"quark:photos", "quark", "photos"},
		{"yun139:media", "yun139", "media"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			gotType, gotName := splitTypeMount(c.in)
			if gotType != c.wantType || gotName != c.wantName {
				t.Errorf("got (%q,%q), want (%q,%q)", gotType, gotName, c.wantType, c.wantName)
			}
		})
	}
}

func TestStreamRegistry_Lifecycle(t *testing.T) {
	r := newStreamRegistry()

	id := r.put(noopReadCloser{})
	if id == "" {
		t.Fatal("empty id")
	}

	if _, ok := r.get(id); !ok {
		t.Fatal("not found after put")
	}

	if _, ok := r.delete(id); !ok {
		t.Fatal("delete returned ok=false")
	}
	if _, ok := r.get(id); ok {
		t.Fatal("still found after delete")
	}
}

type fakeProvider struct{}

func (fakeProvider) Get(string) (string, error)    { return "", nil }
func (fakeProvider) Set(string, string) error      { return nil }
func (fakeProvider) Delete(string) error           { return nil }

type noopReadCloser struct{}

func (noopReadCloser) Read(p []byte) (int, error) { return 0, nil }
func (noopReadCloser) Close() error               { return nil }
