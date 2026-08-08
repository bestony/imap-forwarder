package writer

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Message is a human-readable mail summary ready for output.
type Message struct {
	Account string
	UID     uint32
	Date    time.Time
	From    string
	To      string
	Subject string
	Body    string
}

// Writer appends formatted mail records to a text file.
type Writer struct {
	file *os.File
}

// Open opens path for append (creates if missing).
func Open(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open output file %q: %w", path, err)
	}
	return &Writer{file: f}, nil
}

// Append writes one formatted message record.
func (w *Writer) Append(msg Message) error {
	if w == nil || w.file == nil {
		return fmt.Errorf("writer is closed")
	}
	_, err := io.WriteString(w.file, Format(msg))
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// Close flushes and closes the underlying file.
func (w *Writer) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Format renders a message into the mail.txt record layout.
func Format(msg Message) string {
	var b strings.Builder
	b.WriteString(strings.Repeat("=", 80))
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Account : %s\n", display(msg.Account))
	if msg.UID != 0 {
		fmt.Fprintf(&b, "UID     : %d\n", msg.UID)
	} else {
		fmt.Fprintf(&b, "UID     : %s\n", "(unknown)")
	}
	fmt.Fprintf(&b, "Date    : %s\n", formatDate(msg.Date))
	fmt.Fprintf(&b, "From    : %s\n", display(msg.From))
	fmt.Fprintf(&b, "To      : %s\n", display(msg.To))
	fmt.Fprintf(&b, "Subject : %s\n", display(msg.Subject))
	b.WriteString(strings.Repeat("-", 80))
	b.WriteByte('\n')
	body := strings.TrimSpace(msg.Body)
	if body == "" {
		body = "(empty body)"
	}
	b.WriteString(body)
	b.WriteByte('\n')
	b.WriteString(strings.Repeat("=", 80))
	b.WriteString("\n\n")
	return b.String()
}

func display(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(unknown)"
	}
	return s
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "(unknown)"
	}
	return t.Format("2006-01-02 15:04:05 -0700")
}
