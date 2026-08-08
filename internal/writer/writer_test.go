package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	msg := Message{
		Account: "work",
		UID:     42,
		Date:    time.Date(2026, 8, 8, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		From:    "Alice <alice@example.com>",
		To:      "Bob <bob@example.com>",
		Subject: "Hello",
		Body:    "Hi there",
	}
	out := Format(msg)
	for _, want := range []string{
		"Account : work",
		"UID     : 42",
		"From    : Alice <alice@example.com>",
		"To      : Bob <bob@example.com>",
		"Subject : Hello",
		"Hi there",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Format missing %q\n%s", want, out)
		}
	}
}

func TestFormatEmptyFields(t *testing.T) {
	out := Format(Message{})
	if !strings.Contains(out, "UID     : (unknown)") {
		t.Errorf("expected unknown UID:\n%s", out)
	}
	if !strings.Contains(out, "(empty body)") {
		t.Errorf("expected empty body note:\n%s", out)
	}
	if !strings.Contains(out, "From    : (unknown)") {
		t.Errorf("expected unknown From:\n%s", out)
	}
}

func TestAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mail.txt")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Message{Account: "a", UID: 1, Subject: "one", Body: "body1"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Message{Account: "b", UID: 2, Subject: "two", Body: "body2"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "Account : a") || !strings.Contains(text, "Account : b") {
		t.Fatalf("unexpected content:\n%s", text)
	}
	// Append mode: open again should grow the file.
	w2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w2.Append(Message{Account: "c", UID: 3, Subject: "three"}); err != nil {
		t.Fatal(err)
	}
	_ = w2.Close()
	data2, _ := os.ReadFile(path)
	if !strings.Contains(string(data2), "Account : c") {
		t.Fatal("append did not preserve previous content / write new")
	}
}
