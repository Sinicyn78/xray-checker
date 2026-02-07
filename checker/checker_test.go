package checker

import (
	"testing"

	"xray-checker/metrics"
	"xray-checker/models"
)

func TestGetProxyStatusByStableIDWithDuplicateNames(t *testing.T) {
	p1 := &models.ProxyConfig{
		Protocol: "vless",
		Server:   "1.1.1.1",
		Port:     443,
		Name:     "dup",
		UUID:     "11111111-1111-1111-1111-111111111111",
		SubName:  "s1",
	}
	p2 := &models.ProxyConfig{
		Protocol: "vless",
		Server:   "2.2.2.2",
		Port:     443,
		Name:     "dup",
		UUID:     "22222222-2222-2222-2222-222222222222",
		SubName:  "s2",
	}
	p1.StableID = p1.GenerateStableID()
	p2.StableID = p2.GenerateStableID()

	pc := NewProxyChecker([]*models.ProxyConfig{p1, p2}, 10000, "http://127.0.0.1:1", 1, "http://example.com", "", 1, 1, "status", 2)
	pc.currentMetrics.Store(metricKeyForProxy(p1), true)
	pc.latencyMetrics.Store(metricKeyForProxy(p1), badLatencyThreshold/2)
	pc.currentMetrics.Store(metricKeyForProxy(p2), false)
	pc.latencyMetrics.Store(metricKeyForProxy(p2), badLatencyThreshold*2)

	s1, _, err := pc.GetProxyStatusByStableID(p1.StableID)
	if err != nil {
		t.Fatalf("unexpected error for p1: %v", err)
	}
	s2, _, err := pc.GetProxyStatusByStableID(p2.StableID)
	if err != nil {
		t.Fatalf("unexpected error for p2: %v", err)
	}

	if !s1 || s2 {
		t.Fatalf("unexpected statuses: p1=%v p2=%v", s1, s2)
	}
}

func TestCheckAllProxiesStatusModeDoesNotRequireCurrentIP(t *testing.T) {
	metrics.InitMetrics("test")

	p := &models.ProxyConfig{
		Protocol: "vless",
		Server:   "1.1.1.1",
		Port:     443,
		Name:     "p1",
		UUID:     "11111111-1111-1111-1111-111111111111",
	}
	p.StableID = p.GenerateStableID()

	pc := NewProxyChecker(
		[]*models.ProxyConfig{p},
		10000,
		"http://127.0.0.1:1/should-not-be-called",
		1,
		"http://example.com",
		"",
		1,
		1,
		"status",
		2,
	)

	pc.CheckAllProxies()

	if _, ok := pc.currentMetrics.Load(metricKeyForProxy(p)); !ok {
		t.Fatal("expected status metric to be recorded in status mode")
	}
}
