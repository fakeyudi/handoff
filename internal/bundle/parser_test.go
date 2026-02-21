package bundle

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Unit tests for parser error conditions.

func TestMarkdownParser_PlainMarkdownWithoutSentinel(t *testing.T) {
	p := &MarkdownParser{}

	plainMarkdown := `# Some Document

This is just a regular Markdown file with no handoff sentinel.

## Section

- item 1
- item 2
`
	_, err := p.Parse([]byte(plainMarkdown))
	if err == nil {
		t.Fatal("expected error for plain Markdown without sentinel, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid handoff bundle") {
		t.Errorf("expected error to contain 'not a valid handoff bundle', got: %q", err.Error())
	}
}

func TestMarkdownParser_CorruptedBase64Payload(t *testing.T) {
	p := &MarkdownParser{}

	// Has the version sentinel but the base64 payload is garbage.
	corrupted := `<!-- handoff-bundle-version: 1 -->
<!-- handoff-data: !!!not-valid-base64!!! -->

# Handoff
`
	_, err := p.Parse([]byte(corrupted))
	if err == nil {
		t.Fatal("expected error for corrupted base64 payload, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid handoff bundle") {
		t.Errorf("expected error to contain 'not a valid handoff bundle', got: %q", err.Error())
	}
}

func TestMarkdownParser_MissingDataPayload(t *testing.T) {
	p := &MarkdownParser{}

	// Has the version sentinel but no handoff-data comment.
	noData := `<!-- handoff-bundle-version: 1 -->

# Handoff

Some content but no data payload.
`
	_, err := p.Parse([]byte(noData))
	if err == nil {
		t.Fatal("expected error when data payload is missing, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid handoff bundle") {
		t.Errorf("expected error to contain 'not a valid handoff bundle', got: %q", err.Error())
	}
}

func TestMarkdownParser_ValidBase64ButInvalidJSON(t *testing.T) {
	p := &MarkdownParser{}

	// Valid base64 but the decoded content is not valid JSON.
	badJSON := base64.StdEncoding.EncodeToString([]byte("this is not json {{{"))
	content := "<!-- handoff-bundle-version: 1 -->\n<!-- handoff-data: " + badJSON + " -->\n\n# Handoff\n"

	_, err := p.Parse([]byte(content))
	if err == nil {
		t.Fatal("expected error for valid base64 but invalid embedded JSON, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid handoff bundle") {
		t.Errorf("expected error to contain 'not a valid handoff bundle', got: %q", err.Error())
	}
}

func TestJSONParser_MalformedJSON(t *testing.T) {
	p := &JSONParser{}

	cases := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"truncated object", `{"session": {`},
		{"plain text", "not json at all"},
		{"array instead of object", `[1, 2, 3]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.Parse([]byte(tc.input))
			if err == nil {
				t.Fatalf("expected error for malformed JSON input %q, got nil", tc.input)
			}
			if !strings.Contains(err.Error(), "failed to parse JSON bundle") {
				t.Errorf("expected descriptive error containing 'failed to parse JSON bundle', got: %q", err.Error())
			}
		})
	}
}
