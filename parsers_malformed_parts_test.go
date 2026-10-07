package letters_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mnako/letters"
)

// A multipart/mixed message whose second part is multipart/related without a
// boundary parameter. Before the guard in parsePart, NextPart on the nested
// reader failed forever without consuming input and the skip loop never
// returned.
const nestedMultipartWithoutBoundary = "From: sender@example.com\r\n" +
	"To: recipient@example.com\r\n" +
	"Subject: nested multipart without boundary\r\n" +
	"Message-ID: <nested-no-boundary@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=\"outer\"\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"before\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/related\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>inner</p>\r\n" +
	"--inner--\r\n" +
	"--outer\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"after\r\n" +
	"--outer--\r\n"

// nestedMultipartLabelledQuotedPrintable mirrors the structure Tustena CRM
// produces: multipart/mixed > multipart/related > multipart/alternative, with
// both containers labelled quoted-printable. multipart.Reader.NextPart wraps a
// part labelled that way in a quotedprintable.Reader, which removes the soft
// line breaks of the soft-wrapped html inside, yielding a run of more than
// 4096 bytes without a newline. The multipart reader of the middle container
// then fails its part with "bufio: buffer full", that error is sticky on the
// part, and every further NextPart on the innermost reader returns it without
// advancing. A skip-and-retry loop over NextPart never terminates.
func nestedMultipartLabelledQuotedPrintable() string {
	var html strings.Builder
	html.WriteString("<p>")
	for range 80 {
		html.WriteString(strings.Repeat("a", 75))
		html.WriteString("=\r\n")
	}
	html.WriteString("inner</p>\r\n")

	return "From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: nested multipart labelled quoted-printable\r\n" +
		"Message-ID: <nested-qp@example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"--===outer_1\"\r\n" +
		"\r\n" +
		"----===outer_1\r\n" +
		"Content-Type: multipart/related; boundary=\"--===inner_2\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"----===inner_2\r\n" +
		"Content-Type: multipart/alternative; boundary=\"--===inner_3\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"----===inner_3\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"caff=C3=A8\r\n" +
		"----===inner_3\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		html.String() +
		"----===inner_3--\r\n" +
		"----===inner_2\r\n" +
		"Content-Type: image/png; name=\"logo.png\"\r\n" +
		"Content-ID: <logo@example.com>\r\n" +
		"Content-Disposition: inline; filename=\"logo.png\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"iVBORw0KGgo=\r\n" +
		"----===inner_2--\r\n" +
		"----===outer_1\r\n" +
		"Content-Type: image/png; name=\"a.png\"\r\n" +
		"Content-Disposition: attachment; filename=\"a.png\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"iVBORw0KGgo=\r\n" +
		"----===outer_1--\r\n"
}

func TestParseNestedMultipartLabelledQuotedPrintable(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser(letters.WithSkipMalformedParts(true))
	email, skipped, err := parseWithDeadline(t, parser, nestedMultipartLabelledQuotedPrintable())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped {
		t.Fatal("expected no part to be skipped")
	}
	if !strings.Contains(email.Text, "caffè") {
		t.Fatalf("expected quoted-printable text part decoded, got %q", email.Text)
	}
	if !strings.HasPrefix(email.HTML, "<p>aaaa") || !strings.HasSuffix(strings.TrimSpace(email.HTML), "inner</p>") {
		t.Fatalf("expected nested html part with soft breaks joined, got %q", email.HTML)
	}
	if len(email.InlineFiles) != 1 {
		t.Fatalf("expected the inline image inside the nested container, got %d", len(email.InlineFiles))
	}
	if len(email.AttachedFiles) != 1 {
		t.Fatalf("expected the attachment after the nested container, got %d", len(email.AttachedFiles))
	}
}

func parseWithDeadline(t *testing.T, parser *letters.EmailParser, raw string) (letters.Email, bool, error) {
	t.Helper()

	type result struct {
		email   letters.Email
		skipped bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		email, skipped, err := parser.Parse(strings.NewReader(raw))
		done <- result{email: email, skipped: skipped, err: err}
	}()

	select {
	case r := <-done:
		return r.email, r.skipped, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("Parse did not return within 5s")
		return letters.Email{}, false, nil
	}
}

func TestParseNestedMultipartWithoutBoundarySkipsPart(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser(letters.WithSkipMalformedParts(true))
	email, skipped, err := parseWithDeadline(t, parser, nestedMultipartWithoutBoundary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !skipped {
		t.Fatal("expected the boundary-less part to be reported as skipped")
	}
	if !strings.Contains(email.Text, "before") || !strings.Contains(email.Text, "after") {
		t.Fatalf("expected sibling parts to survive, got text %q", email.Text)
	}
	if email.HTML != "" {
		t.Fatalf("expected the boundary-less part to be dropped, got html %q", email.HTML)
	}
}

func TestParseNestedMultipartWithoutBoundaryFailsWhenNotSkipping(t *testing.T) {
	t.Parallel()

	parser := letters.NewEmailParser()
	_, _, err := parseWithDeadline(t, parser, nestedMultipartWithoutBoundary)
	if err == nil {
		t.Fatal("expected an error for a multipart part without boundary")
	}
	if !strings.Contains(err.Error(), "has no boundary") {
		t.Fatalf("unexpected error: %v", err)
	}
}
