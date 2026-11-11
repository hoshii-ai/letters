package letters_test

import (
	"strings"
	"testing"

	"github.com/mnako/letters"
)

const messageWithMalformedFrom = "From: <not an address\r\n" +
	"To: recipient@example.com\r\n" +
	"Subject: malformed from\r\n" +
	"Message-ID: <malformed-from@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"body text\r\n"

const messageWithTwoMalformedHeaders = "Sender: sender@example.com\r\n" +
	"From: <not an address\r\n" +
	"Reply-To: reply@example.com\r\n" +
	"To: <not an address\r\n" +
	"Cc: copy@example.com\r\n" +
	"Subject: two malformed headers\r\n" +
	"Message-ID: <two-malformed-headers@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"body text\r\n"

const messageWithControlByteInFrom = "From: Tr\x01n <sender@example.com>\r\n" +
	"To: recipient@example.com\r\n" +
	"Subject: control byte in display name\r\n" +
	"Message-ID: <control-byte-in-from@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"body text\r\n"

const messageWithMalformedContentType = "From: sender@example.com\r\n" +
	"To: recipient@example.com\r\n" +
	"Subject: malformed content type\r\n" +
	"Message-ID: <malformed-content-type@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset\r\n" +
	"\r\n" +
	"body text\r\n"

func TestParseMalformedFromSkipsOnlyThatHeader(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser(letters.WithSkipMalformedHeaders(true))
	email, skipped, err := parseWithDeadline(t, parser, messageWithMalformedFrom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Fatal("expected the malformed From header to be reported as skipped")
	}
	if len(email.Headers.From) != 0 {
		t.Fatalf("expected From to stay empty, got %v", email.Headers.From)
	}
	if len(email.Headers.To) != 1 || email.Headers.To[0].Address != "recipient@example.com" {
		t.Fatalf("expected To to survive, got %v", email.Headers.To)
	}
	if email.Headers.Subject != "malformed from" {
		t.Fatalf("expected Subject to survive, got %q", email.Headers.Subject)
	}
	if !strings.Contains(email.Text, "body text") {
		t.Fatalf("expected body to survive, got text %q", email.Text)
	}
}

func TestParseTwoMalformedHeadersSkipsBothAndKeepsValidOnes(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser(letters.WithSkipMalformedHeaders(true))
	email, skipped, err := parseWithDeadline(t, parser, messageWithTwoMalformedHeaders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Fatal("expected the malformed headers to be reported as skipped")
	}
	if len(email.Headers.From) != 0 {
		t.Fatalf("expected From to stay empty, got %v", email.Headers.From)
	}
	if len(email.Headers.To) != 0 {
		t.Fatalf("expected To to stay empty, got %v", email.Headers.To)
	}
	if email.Headers.Sender == nil || email.Headers.Sender.Address != "sender@example.com" {
		t.Fatalf("expected Sender to survive, got %v", email.Headers.Sender)
	}
	if len(email.Headers.ReplyTo) != 1 || email.Headers.ReplyTo[0].Address != "reply@example.com" {
		t.Fatalf("expected Reply-To to survive, got %v", email.Headers.ReplyTo)
	}
	if len(email.Headers.Cc) != 1 || email.Headers.Cc[0].Address != "copy@example.com" {
		t.Fatalf("expected Cc to survive, got %v", email.Headers.Cc)
	}
	if email.Headers.Subject != "two malformed headers" {
		t.Fatalf("expected Subject to survive, got %q", email.Headers.Subject)
	}
}

func TestParseMalformedFromFailsWhenNotSkipping(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser()
	_, _, err := parseWithDeadline(t, parser, messageWithMalformedFrom)
	if err == nil {
		t.Fatal("expected an error for a malformed From header")
	}
	if !strings.Contains(err.Error(), "cannot parse From header") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseControlByteInDisplayNameSkipsOnlyFrom(t *testing.T) {
	t.Parallel()

	_, _, err := parseWithDeadline(t, letters.NewEmailParser(), messageWithControlByteInFrom)
	if err == nil {
		t.Fatal("expected the default parser to reject a control byte in a display name")
	}

	parser := letters.NewEmailParser(letters.WithSkipMalformedHeaders(true))
	email, skipped, err := parseWithDeadline(t, parser, messageWithControlByteInFrom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Fatal("expected the From header to be reported as skipped")
	}
	if len(email.Headers.From) != 0 {
		t.Fatalf("expected From to stay empty, got %v", email.Headers.From)
	}
	if len(email.Headers.To) != 1 || email.Headers.To[0].Address != "recipient@example.com" {
		t.Fatalf("expected To to survive, got %v", email.Headers.To)
	}
	if email.Headers.Subject != "control byte in display name" {
		t.Fatalf("expected Subject to survive, got %q", email.Headers.Subject)
	}
	if !strings.Contains(email.Text, "body text") {
		t.Fatalf("expected body to survive, got text %q", email.Text)
	}
}

func TestParseMalformedContentTypeSkipsOnlyThatHeader(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser(letters.WithSkipMalformedHeaders(true))
	email, skipped, err := parseWithDeadline(t, parser, messageWithMalformedContentType)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Fatal("expected the malformed Content-Type header to be reported as skipped")
	}
	if email.Headers.ContentType.ContentType != "" {
		t.Fatalf("expected Content-Type to stay empty, got %q", email.Headers.ContentType.ContentType)
	}
	if len(email.Headers.To) != 1 || email.Headers.To[0].Address != "recipient@example.com" {
		t.Fatalf("expected To to survive, got %v", email.Headers.To)
	}
	if email.Headers.Subject != "malformed content type" {
		t.Fatalf("expected Subject to survive, got %q", email.Headers.Subject)
	}
}
