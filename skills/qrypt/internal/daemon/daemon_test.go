package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
	"github.com/yinzhenyu/skills/qrypt/internal/rpc"
)

var _ rpc.RPCHost = (*Daemon)(nil) // compile-time check


func newTestDaemon(t *testing.T) (*Daemon, *rpc.WSClient, string) {
	t.Helper()

	dataDir := t.TempDir()
	socketPath := filepath.Join(os.TempDir(), "qrypt-"+filepath.Base(dataDir)+".sock")

	cfg := &config.Config{
		Mounts: []config.MountInstance{{
			Name:   "test",
			Type:   "localfs",
			Params: config.MountParams{LocalRoot: dataDir},
			Encryption: &config.EncryptionConfig{
				Password: "test",
			},
		}},
	}

	d := NewDaemon(cfg, "test")
	srv := rpc.NewWSServer(d, socketPath)
	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { srv.Stop(); os.Remove(socketPath) })

	client, err := rpc.DialWS(socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(client.Close)

	return d, client, dataDir
}

func rpcCall(t *testing.T, client *rpc.WSClient, method string, params interface{}) *protocol.Response {
	t.Helper()
	resp, err := client.Call(method, params)
	if err != nil {
		t.Fatalf("RPC %s: %v", method, err)
	}
	if resp.Error != nil {
		t.Fatalf("RPC %s error: %s (code %d)", method, resp.Error.Message, resp.Error.Code)
	}
	return resp
}

func unmarshalResult(t *testing.T, resp *protocol.Response, v interface{}) {
	t.Helper()
	data, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestDaemonIPC_Status(t *testing.T) {
	_, client, _ := newTestDaemon(t)
	resp := rpcCall(t, client, "status", nil)
	var status protocol.DaemonStatus
	unmarshalResult(t, resp, &status)
	if status.Version != "test" {
		t.Errorf("version = %q, want %q", status.Version, "test")
	}
}

func TestDaemonIPC_MkdirAndListDir(t *testing.T) {
	_, client, _ := newTestDaemon(t)

	rpcCall(t, client, "mkdir", protocol.MkdirParams{
		MountName: "test",
		Path:      "/subdir",
		Parents:   true,
	})

	resp := rpcCall(t, client, "list_dir", protocol.ListDirParams{MountName: "test", Path: "/"})
	var list protocol.ListDirResult
	unmarshalResult(t, resp, &list)
	if len(list.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list.Entries))
	}
	if list.Entries[0].DecName != "subdir" {
		t.Errorf("entry = %q, want %q", list.Entries[0].DecName, "subdir")
	}
	if !list.Entries[0].IsDir {
		t.Error("expected directory")
	}
}

func TestDaemonIPC_ReadFile(t *testing.T) {
	_, client, dataDir := newTestDaemon(t)
	os.WriteFile(filepath.Join(dataDir, "hello.txt"), []byte("hello"), 0644)

	resp := rpcCall(t, client, "list_dir", protocol.ListDirParams{MountName: "test", Path: "/"})
	var list protocol.ListDirResult
	unmarshalResult(t, resp, &list)
	if len(list.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list.Entries))
	}
}

func TestDaemonIPC_Remove(t *testing.T) {
	_, client, dataDir := newTestDaemon(t)
	os.WriteFile(filepath.Join(dataDir, "del.txt"), []byte("x"), 0644)

	rpcCall(t, client, "remove", protocol.RemoveParams{MountName: "test", Path: "/del.txt"})

	resp := rpcCall(t, client, "list_dir", protocol.ListDirParams{MountName: "test", Path: "/"})
	var list protocol.ListDirResult
	unmarshalResult(t, resp, &list)
	if len(list.Entries) != 0 {
		t.Errorf("expected 0 after remove, got %d", len(list.Entries))
	}
}

func TestDaemonIPC_Move(t *testing.T) {
	_, client, dataDir := newTestDaemon(t)
	os.WriteFile(filepath.Join(dataDir, "src.txt"), []byte("x"), 0644)

	rpcCall(t, client, "move", protocol.MoveParams{
		MountName: "test", SrcPath: "/src.txt", DstPath: "/dst.txt",
	})

	resp := rpcCall(t, client, "list_dir", protocol.ListDirParams{MountName: "test", Path: "/"})
	var list protocol.ListDirResult
	unmarshalResult(t, resp, &list)
	if len(list.Entries) != 1 {
		t.Fatalf("expected 1 after move, got %d", len(list.Entries))
	}
}

func TestDaemonIPC_Find(t *testing.T) {
	_, client, dataDir := newTestDaemon(t)
	os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dataDir, "b.go"), []byte("y"), 0644)

	resp := rpcCall(t, client, "find", protocol.FindParams{
		MountName: "test", Path: "/", Pattern: ".txt",
	})
	var result protocol.FindResult
	unmarshalResult(t, resp, &result)
	if result.Count != 1 {
		t.Errorf("expected 1 match, got %d", result.Count)
	}
}

func TestDaemonIPC_FindNested(t *testing.T) {
	_, client, dataDir := newTestDaemon(t)
	os.MkdirAll(filepath.Join(dataDir, "sub"), 0755)
	os.WriteFile(filepath.Join(dataDir, "root.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dataDir, "sub", "deep.txt"), []byte("y"), 0644)

	resp := rpcCall(t, client, "find", protocol.FindParams{
		MountName: "test", Path: "/", Pattern: ".txt",
	})
	var result protocol.FindResult
	unmarshalResult(t, resp, &result)
	if result.Count != 2 {
		t.Errorf("expected 2 matches, got %d", result.Count)
	}
}

func TestDaemonIPC_ListDirNotFound(t *testing.T) {
	_, client, _ := newTestDaemon(t)
	resp, err := client.Call("list_dir", protocol.ListDirParams{MountName: "test", Path: "/nope"})
	if err == nil && resp.Error == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestDaemonIPC_MountList(t *testing.T) {
	d, client, _ := newTestDaemon(t)
	ctx := context.Background()
	d.Start(ctx, "test")

	resp := rpcCall(t, client, "mount_list", nil)
	var summaries []protocol.MountSummary
	unmarshalResult(t, resp, &summaries)
	if len(summaries) == 0 {
		t.Error("expected mounts")
	}
}
