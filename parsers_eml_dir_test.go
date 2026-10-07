package letters_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnako/letters"
)

// TestParseEmlDir parses every .eml under LETTERS_EML_DIR. Each file must parse within 10s. Skipped when
// the variable is unset, so CI is unaffected.
func TestParseEmlDir(t *testing.T) {
	dir := os.Getenv("LETTERS_EML_DIR")
	if dir == "" {
		t.Skip("LETTERS_EML_DIR not set")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.eml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no .eml files in %s (%v)", dir, err)
	}

	parser := letters.NewEmailParser(
		letters.WithSkipMalformedHeaders(true),
		letters.WithSkipMalformedParts(true),
	)

	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			type result struct {
				email   letters.Email
				skipped bool
				err     error
			}
			done := make(chan result, 1)
			start := time.Now()
			go func() {
				email, skipped, err := parser.Parse(strings.NewReader(string(raw)))
				done <- result{email, skipped, err}
			}()

			select {
			case r := <-done:
				if r.err != nil {
					t.Fatalf("parse error: %v", r.err)
				}
				t.Logf("parsed in %s: skipped=%v text=%dB html=%dB inline=%d attached=%d subject=%q",
					time.Since(start).Round(time.Millisecond), r.skipped,
					len(r.email.Text), len(r.email.HTML),
					len(r.email.InlineFiles), len(r.email.AttachedFiles),
					r.email.Headers.Subject)
			case <-time.After(10 * time.Second):
				t.Fatal("Parse did not return within 10s")
			}
		})
	}
}
