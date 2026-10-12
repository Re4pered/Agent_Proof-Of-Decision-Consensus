package poad

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// LoadTraceIntentLinksVerified checks the exact manifest bytes against an
// externally trusted SHA-256 digest before parsing the JSONL entries. The expected
// digest must come from a trusted/committed source; a digest stored beside an
// attacker-controlled manifest does not authenticate that manifest.
func LoadTraceIntentLinksVerified(reader io.Reader, expectedSHA256 string) ([]TraceIntentLink, error) {
	expected := strings.TrimSpace(expectedSHA256)
	if len(expected) != sha256.Size*2 {
		return nil, fmt.Errorf("expected SHA-256 digest must contain 64 hexadecimal characters")
	}

	if _, err := hex.DecodeString(expected); err != nil {
		return nil, fmt.Errorf("decode expected SHA-256 digest: %w", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read trace intent manifest: %w", err)
	}

	actual := sha256.Sum256(data)
	actualHex := hex.EncodeToString(actual[:])

	if !strings.EqualFold(expected, actualHex) {
		return nil, fmt.Errorf("trace intent manifest SHA-256 mismatch")
	}

	links, err := LoadTraceIntentLinks(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	return links, nil
}
