//go:build e2e

package reporting

import (
	"encoding/json"
	"fmt"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
)

// formatJSON converts TestResult to pretty-printed JSON
func formatJSON(result *e2e.TestResult) (string, error) {
	// Marshal with indentation for readability
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(data), nil
}
