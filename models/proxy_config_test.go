package models

import "testing"

func TestGenerateStableIDIncludesTransportCriticalFields(t *testing.T) {
	base := &ProxyConfig{
		Protocol:  "vless",
		Server:    "1.1.1.1",
		Port:      443,
		UUID:      "11111111-1111-1111-1111-111111111111",
		Type:      "ws",
		Security:  "tls",
		PublicKey: "pub-key",
		SNI:       "example.com",
	}

	shortIDA := *base
	shortIDA.ShortID = "aaa"
	shortIDB := *base
	shortIDB.ShortID = "bbb"
	if shortIDA.GenerateStableID() == shortIDB.GenerateStableID() {
		t.Fatalf("expected different stable IDs for different short IDs")
	}

	pathA := *base
	pathA.Path = "/a"
	pathB := *base
	pathB.Path = "/b"
	if pathA.GenerateStableID() == pathB.GenerateStableID() {
		t.Fatalf("expected different stable IDs for different paths")
	}

	hostA := *base
	hostA.Host = "a.example.com"
	hostB := *base
	hostB.Host = "b.example.com"
	if hostA.GenerateStableID() == hostB.GenerateStableID() {
		t.Fatalf("expected different stable IDs for different hosts")
	}
}
