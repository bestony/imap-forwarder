package fetcher

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/mail"
	"strings"
	"unicode"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"

	"github.com/bestony/imap-forwarder/internal/config"
	"github.com/bestony/imap-forwarder/internal/writer"
)

// FetchAll connects to the account, streams every message in the mailbox, and
// invokes handle for each parsed summary. handle is called sequentially.
func FetchAll(account config.Account, maxBodyBytes int64, handle func(writer.Message) error) (int, error) {
	log := slog.With("account", account.Name, "host", account.Host, "mailbox", account.Mailbox)

	options := &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}

	log.Debug("dialing IMAP server", "address", account.Address(), "tls_mode", account.TLSMode)
	c, err := dial(account, options)
	if err != nil {
		return 0, fmt.Errorf("dial %s: %w", account.Address(), err)
	}
	defer func() {
		if closeErr := c.Close(); closeErr != nil {
			log.Debug("client close error", "err", closeErr)
		}
	}()

	if account.TLSMode == "insecure" {
		log.Warn("using insecure IMAP connection (no TLS)")
	}

	log.Debug("logging in", "username", account.Username)
	if err := c.Login(account.Username, account.Password).Wait(); err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}
	defer func() {
		if logoutErr := c.Logout().Wait(); logoutErr != nil {
			log.Debug("logout error", "err", logoutErr)
		}
	}()

	selected, err := c.Select(account.Mailbox, nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("select mailbox %q: %w", account.Mailbox, err)
	}
	log.Info("mailbox selected", "num_messages", selected.NumMessages)
	if selected.NumMessages == 0 {
		return 0, nil
	}

	var seqSet imap.SeqSet
	seqSet.AddRange(1, selected.NumMessages)

	bodySection := &imap.FetchItemBodySection{}
	fetchOptions := &imap.FetchOptions{
		UID:         true,
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	log.Debug("fetching messages", "from", 1, "to", selected.NumMessages)
	fetchCmd := c.Fetch(seqSet, fetchOptions)

	count := 0
	for {
		msgData := fetchCmd.Next()
		if msgData == nil {
			break
		}

		buf, err := msgData.Collect()
		if err != nil {
			_ = fetchCmd.Close()
			return count, fmt.Errorf("collect message data: %w", err)
		}

		summary, err := parseMessage(account.Name, buf, bodySection, maxBodyBytes)
		if err != nil {
			log.Warn("failed to parse message; writing headers only", "uid", buf.UID, "err", err)
			summary = envelopeOnly(account.Name, buf)
			summary.Body = fmt.Sprintf("(failed to parse body: %v)", err)
		}

		if err := handle(summary); err != nil {
			_ = fetchCmd.Close()
			return count, fmt.Errorf("handle message uid=%d: %w", summary.UID, err)
		}
		count++
		log.Info("message written", "uid", summary.UID, "subject", summary.Subject)
	}

	if err := fetchCmd.Close(); err != nil {
		return count, fmt.Errorf("fetch: %w", err)
	}

	log.Info("fetch completed", "written", count)
	return count, nil
}

func dial(account config.Account, options *imapclient.Options) (*imapclient.Client, error) {
	addr := account.Address()
	switch account.TLSMode {
	case "tls":
		return imapclient.DialTLS(addr, options)
	case "starttls":
		return imapclient.DialStartTLS(addr, options)
	case "insecure":
		return imapclient.DialInsecure(addr, options)
	default:
		return nil, fmt.Errorf("unsupported tls_mode %q", account.TLSMode)
	}
}

func parseMessage(accountName string, buf *imapclient.FetchMessageBuffer, bodySection *imap.FetchItemBodySection, maxBodyBytes int64) (writer.Message, error) {
	msg := envelopeOnly(accountName, buf)

	raw := buf.FindBodySection(bodySection)
	if raw == nil {
		msg.Body = "(no body section returned by server)"
		return msg, nil
	}

	mr, err := gomail.CreateReader(strings.NewReader(string(raw)))
	if err != nil {
		return msg, fmt.Errorf("create mail reader: %w", err)
	}

	// Prefer header fields from the full message when available.
	if date, err := mr.Header.Date(); err == nil && !date.IsZero() {
		msg.Date = date
	}
	if subj, err := mr.Header.Text("Subject"); err == nil && strings.TrimSpace(subj) != "" {
		msg.Subject = subj
	}
	if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
		msg.From = formatAddresses(from)
	}
	if to, err := mr.Header.AddressList("To"); err == nil && len(to) > 0 {
		msg.To = formatAddresses(to)
	}

	plain, html, err := extractBodies(mr, maxBodyBytes)
	if err != nil {
		return msg, err
	}
	switch {
	case strings.TrimSpace(plain) != "":
		msg.Body = plain
	case strings.TrimSpace(html) != "":
		msg.Body = stripHTML(html)
	default:
		msg.Body = "(no text body)"
	}
	return msg, nil
}

func envelopeOnly(accountName string, buf *imapclient.FetchMessageBuffer) writer.Message {
	msg := writer.Message{
		Account: accountName,
		UID:     uint32(buf.UID),
	}
	if buf.Envelope != nil {
		env := buf.Envelope
		msg.Subject = env.Subject
		if !env.Date.IsZero() {
			msg.Date = env.Date
		}
		msg.From = formatIMAPAddresses(env.From)
		msg.To = formatIMAPAddresses(env.To)
	}
	return msg
}

func extractBodies(mr *gomail.Reader, maxBodyBytes int64) (plain, html string, err error) {
	for {
		part, partErr := mr.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			return plain, html, fmt.Errorf("read part: %w", partErr)
		}

		switch h := part.Header.(type) {
		case *gomail.InlineHeader:
			ct, _, _ := h.ContentType()
			limited := io.LimitReader(part.Body, maxBodyBytes+1)
			data, readErr := io.ReadAll(limited)
			if readErr != nil {
				return plain, html, fmt.Errorf("read inline body: %w", readErr)
			}
			if int64(len(data)) > maxBodyBytes {
				data = data[:maxBodyBytes]
			}
			text := string(data)
			switch {
			case strings.HasPrefix(strings.ToLower(ct), "text/plain") && plain == "":
				plain = text
			case strings.HasPrefix(strings.ToLower(ct), "text/html") && html == "":
				html = text
			case ct == "" && plain == "":
				// Some servers omit content-type on simple messages.
				plain = text
			}
		case *gomail.AttachmentHeader:
			// Attachments are intentionally ignored for mail.txt summaries.
			_, _ = io.Copy(io.Discard, part.Body)
		default:
			_, _ = io.Copy(io.Discard, part.Body)
		}
	}
	return plain, html, nil
}

func formatIMAPAddresses(addrs []imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, a.Addr())
	}
	return strings.Join(parts, ", ")
}

func formatAddresses(addrs []*mail.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a == nil {
			continue
		}
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ", ")
}

// stripHTML is a lightweight tag stripper for fallback HTML bodies.
func stripHTML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	// Collapse runs of whitespace for readability.
	fields := strings.FieldsFunc(b.String(), func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	var out strings.Builder
	for i, line := range fields {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Normalize internal spaces.
		line = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return ' '
			}
			return r
		}, line)
		line = strings.Join(strings.Fields(line), " ")
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(line)
	}
	return strings.TrimSpace(out.String())
}
