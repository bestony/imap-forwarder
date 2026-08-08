package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidMultiAccount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
output_file = "out.txt"
max_body_bytes = 1024

[[accounts]]
name = "work"
host = "imap.example.com"
username = "a@example.com"
password = "secret"

[[accounts]]
name = "personal"
host = "imap.gmail.com"
port = 993
username = "b@example.com"
password = "app-pass"
mailbox = "INBOX"
tls_mode = "starttls"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OutputFile != "out.txt" {
		t.Fatalf("OutputFile = %q", cfg.OutputFile)
	}
	if cfg.MaxBodyBytes != 1024 {
		t.Fatalf("MaxBodyBytes = %d", cfg.MaxBodyBytes)
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("len(Accounts) = %d", len(cfg.Accounts))
	}

	a0 := cfg.Accounts[0]
	if a0.Port != DefaultPort {
		t.Errorf("default port = %d, want %d", a0.Port, DefaultPort)
	}
	if a0.Mailbox != DefaultMailbox {
		t.Errorf("default mailbox = %q, want %q", a0.Mailbox, DefaultMailbox)
	}
	if a0.TLSMode != DefaultTLSMode {
		t.Errorf("default tls_mode = %q, want %q", a0.TLSMode, DefaultTLSMode)
	}
	if a0.Address() != "imap.example.com:993" {
		t.Errorf("Address = %q", a0.Address())
	}

	a1 := cfg.Accounts[1]
	if a1.TLSMode != "starttls" {
		t.Errorf("tls_mode = %q, want starttls", a1.TLSMode)
	}
}

func TestLoadMissingAccounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`output_file = "mail.txt"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing accounts")
	}
}

func TestLoadMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[[accounts]]
name = "x"
host = "imap.example.com"
username = "u"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing password")
	}
}

func TestLoadInvalidTLSMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[[accounts]]
name = "x"
host = "imap.example.com"
username = "u"
password = "p"
tls_mode = "something"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid tls_mode")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
