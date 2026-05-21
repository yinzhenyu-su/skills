package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func makeToolCmd(cfgPath string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("salt", "", "")
	cmd.Flags().String("mount", "", "")
	return cmd
}

func writeToolTestConfig(t *testing.T, dir, password, salt string) string {
	t.Helper()
	cfgPath := filepath.Join(dir, "qrypt.toml")
	cfg := "version = \"1\"\n\n[defaults.encryption]\npassword = \"" + password + "\"\nsalt = \"" + salt + "\"\n\n[[mounts]]\nname = \"test\"\ndrive_type = \"localfs\"\nmount_point = \"" + dir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

// runWithCapturedStdout runs fn and returns everything written to os.Stdout.
// The pipe is read concurrently to avoid deadlock on full pipe buffers.
func runWithCapturedStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	done := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()

	fn()

	w.Close()
	os.Stdout = old
	return <-done
}

func TestToolEncryptDecryptFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeToolTestConfig(t, dir, "test-password", "test-salt")
	cmd := makeToolCmd(cfgPath)

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"small", []byte("hello qrypt")},
		{"empty", []byte{}},
		{"single-block", bytes.Repeat([]byte("A"), 64*1024)},
		{"multi-block", bytes.Repeat([]byte("B"), 64*1024*3+1000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plainPath := filepath.Join(dir, "input-"+tt.name)
			if err := os.WriteFile(plainPath, tt.plaintext, 0644); err != nil {
				t.Fatal(err)
			}

			encrypted := runWithCapturedStdout(t, func() {
				encryptFile(cmd, plainPath)
			})

			encPath := filepath.Join(dir, "enc-"+tt.name)
			if err := os.WriteFile(encPath, encrypted, 0644); err != nil {
				t.Fatal(err)
			}

			decrypted := runWithCapturedStdout(t, func() {
				decryptFile(cmd, encPath)
			})

			if !bytes.Equal(tt.plaintext, decrypted) {
				t.Errorf("round-trip mismatch: got %d bytes, want %d bytes", len(decrypted), len(tt.plaintext))
			}
		})
	}
}

func TestToolEncryptDecryptPipe(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeToolTestConfig(t, dir, "pipe-password", "pipe-salt")
	cmd := makeToolCmd(cfgPath)

	plaintext := []byte("hello from stdin pipe")

	encrypted := runWithCapturedStdout(t, func() {
		stdinR, stdinW, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stdinW.Write(plaintext); err != nil {
			t.Fatal(err)
		}
		stdinW.Close()

		oldStdin := os.Stdin
		os.Stdin = stdinR
		defer func() { os.Stdin = oldStdin }()

		encryptFile(cmd, "-")
	})

	decrypted := runWithCapturedStdout(t, func() {
		stdinR2, stdinW2, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stdinW2.Write(encrypted); err != nil {
			t.Fatal(err)
		}
		stdinW2.Close()

		oldStdin := os.Stdin
		os.Stdin = stdinR2
		defer func() { os.Stdin = oldStdin }()

		decryptFile(cmd, "-")
	})

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("pipe round-trip mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}
