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

func TestConvertOutboundUsesOriginalNameAndSourceLine(t *testing.T) {
	p := NewParser()
	raw := []byte(`{
		"protocol":"vless",
		"tag":"lib-tag-name",
		"sendThrough":"lib-sendthrough-name",
		"settings":{
			"address":"1.1.1.1",
			"port":443,
			"id":"11111111-1111-1111-1111-111111111111",
			"encryption":"none"
		},
		"streamSettings":{
			"network":"ws",
			"security":"tls",
			"tlsSettings":{"serverName":"example.com"}
		}
	}`)

	originalData := map[string][]*originalLinkData{
		"1.1.1.1:443": {
			{
				Protocol: "vless",
				UUID:     "11111111-1111-1111-1111-111111111111",
				Name:     "Original Name",
				RawLine:  "vless://original-line",
			},
		},
	}

	pc, err := p.convertOutbound(raw, 0, originalData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc == nil {
		t.Fatalf("expected proxy config, got nil")
	}
	if pc.Name != "Original Name" {
		t.Fatalf("expected name from original link, got %q", pc.Name)
	}
	if pc.SourceLine != "vless://original-line" {
		t.Fatalf("expected source line from original link, got %q", pc.SourceLine)
	}
}
