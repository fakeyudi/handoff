package bundle

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// BundleParser deserializes a context bundle file back into structured data.
type BundleParser interface {
	Parse(data []byte) (*ContextBundle, error)
}

// JSONParser parses a JSON-encoded ContextBundle.
type JSONParser struct{}

func (p *JSONParser) Parse(data []byte) (*ContextBundle, error) {
	var bundle ContextBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse JSON bundle: %w", err)
	}
	return &bundle, nil
}

// MarkdownParser parses a Markdown-rendered ContextBundle by extracting the
// embedded base64 JSON payload from the sentinel comments.
type MarkdownParser struct{}

func (p *MarkdownParser) Parse(data []byte) (*ContextBundle, error) {
	content := string(data)

	// Require the version sentinel.
	if !strings.Contains(content, "<!-- handoff-bundle-version: 1 -->") {
		return nil, fmt.Errorf("not a valid handoff bundle: missing version sentinel")
	}

	// Extract the base64 payload from <!-- handoff-data: <base64> -->.
	const prefix = "<!-- handoff-data: "
	const suffix = " -->"
	start := strings.Index(content, prefix)
	if start == -1 {
		return nil, fmt.Errorf("not a valid handoff bundle: missing data payload")
	}
	start += len(prefix)
	end := strings.Index(content[start:], suffix)
	if end == -1 {
		return nil, fmt.Errorf("not a valid handoff bundle: malformed data payload")
	}
	encoded := content[start : start+end]

	// Base64-decode the payload.
	jsonBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("not a valid handoff bundle: corrupted base64 payload: %w", err)
	}

	// Unmarshal the JSON into a ContextBundle.
	var bundle ContextBundle
	if err := json.Unmarshal(jsonBytes, &bundle); err != nil {
		return nil, fmt.Errorf("not a valid handoff bundle: failed to parse embedded JSON: %w", err)
	}

	return &bundle, nil
}
