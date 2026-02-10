package subscription

import (
	"testing"
	"xray-checker/models"
)

func TestPickBestOriginalCandidatePrefersUUIDAndPath(t *testing.T) {
	candidates := []*originalLinkData{
		{
			Protocol: "vless",
			UUID:     "uuid-1",
			Type:     "ws",
			Security: "tls",
			SNI:      "example.com",
			Path:     "/path-a",
			RawLine:  "line-a",
		},
		{
			Protocol: "vless",
			UUID:     "uuid-2",
			Type:     "ws",
			Security: "tls",
			SNI:      "example.com",
			Path:     "/path-b",
			RawLine:  "line-b",
		},
	}

	pc := &models.ProxyConfig{
		Protocol: "vless",
		UUID:     "uuid-2",
		Type:     "ws",
		Security: "tls",
		SNI:      "example.com",
		Path:     "/path-b",
	}

	idx := pickBestOriginalCandidate(candidates, pc)
	if idx != 1 {
		t.Fatalf("expected candidate 1, got %d", idx)
	}
}
