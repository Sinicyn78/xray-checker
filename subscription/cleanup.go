package subscription

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func RemoveBadConfigsFromFile(filePath string, badLines map[string]bool) (int, int, error) {
	if filePath == "" || len(badLines) == 0 {
		return 0, 0, nil
	}

	rawData, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, err
	}

	parser := NewParser()
	trimmed := strings.TrimSpace(string(rawData))
	if trimmed == "" {
		return 0, 0, nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// JSON configs are not modified here.
		return 0, 0, nil
	}

	decoded := parser.tryDecodeBase64(rawData)
	isBase64 := parser.isLikelyBase64Subscription(rawData, decoded)

	lines := strings.Split(string(decoded), "\n")
	bom := string([]byte{0xEF, 0xBB, 0xBF})

	var kept []string
	removed := 0

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		trim = strings.TrimPrefix(trim, bom)
		if trim == "" {
			continue
		}
		if badLines[trim] {
			removed++
			continue
		}
		kept = append(kept, trim)
	}

	if removed == 0 {
		return 0, len(kept), nil
	}
	if len(kept) == 0 {
		return removed, 0, fmt.Errorf("all configs removed; refusing to write empty file")
	}

	out := strings.Join(kept, "\n")
	if isBase64 {
		out = base64.StdEncoding.EncodeToString([]byte(out))
	}

	if err := os.WriteFile(filePath, []byte(out), 0o644); err != nil {
		return 0, len(kept), err
	}

	return removed, len(kept), nil
}
