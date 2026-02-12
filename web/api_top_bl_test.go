package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"xray-checker/checker"
	"xray-checker/models"
)

var testProxySeq int

func TestSelectTopBLByLatencyFiltersAndSorts(t *testing.T) {
	proxies := []*models.ProxyConfig{
		newTestProxy("BL Alpha", "vless://1"),
		newTestProxy("bl Beta", "vless://2"),
		newTestProxy("Gamma", "vless://3"),
		newTestProxy("BL Delta", "vless://4"),
		newTestProxy("BL EmptyLink", ""),
	}

	status := map[string]struct {
		online  bool
		latency time.Duration
		err     error
	}{
		proxies[0].StableID: {online: true, latency: 120 * time.Millisecond},
		proxies[1].StableID: {online: true, latency: 80 * time.Millisecond},
		proxies[2].StableID: {online: true, latency: 10 * time.Millisecond}, // no BL in name
		proxies[3].StableID: {online: false, latency: 20 * time.Millisecond}, // offline
		proxies[4].StableID: {online: true, latency: 5 * time.Millisecond},   // empty link
	}

	got := selectTopBLByLatency(proxies, func(stableID string) (bool, time.Duration, error) {
		v, ok := status[stableID]
		if !ok {
			return false, 0, fmt.Errorf("unknown stable id")
		}
		return v.online, v.latency, v.err
	}, 10)

	if len(got.proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(got.proxies))
	}

	if got.proxies[0].Name != "bl Beta" {
		t.Fatalf("expected fastest BL proxy to be bl Beta, got %s", got.proxies[0].Name)
	}
	if got.proxies[1].Name != "BL Alpha" {
		t.Fatalf("expected second BL proxy to be BL Alpha, got %s", got.proxies[1].Name)
	}
}

func TestSelectTopBLByLatencyLimit(t *testing.T) {
	proxies := make([]*models.ProxyConfig, 0, 12)
	latencyByID := make(map[string]time.Duration, 12)

	for i := 0; i < 12; i++ {
		p := newTestProxy(fmt.Sprintf("BL Node %02d", i), fmt.Sprintf("vless://%d", i))
		proxies = append(proxies, p)
		latencyByID[p.StableID] = time.Duration(i+1) * time.Millisecond
	}

	got := selectTopBLByLatency(proxies, func(stableID string) (bool, time.Duration, error) {
		return true, latencyByID[stableID], nil
	}, 10)

	if len(got.proxies) != 10 {
		t.Fatalf("expected 10 proxies, got %d", len(got.proxies))
	}

	if got.proxies[0].Name != "BL Node 00" {
		t.Fatalf("expected first proxy BL Node 00, got %s", got.proxies[0].Name)
	}
	if got.proxies[9].Name != "BL Node 09" {
		t.Fatalf("expected tenth proxy BL Node 09, got %s", got.proxies[9].Name)
	}
}

func TestSelectTopBLByLatencyDeduplicatesByUUID(t *testing.T) {
	fast := newTestProxy("BL Fast", "vless://fast")
	slow := newTestProxy("BL Slow", "vless://slow")
	other := newTestProxy("BL Other", "vless://other")

	// Same UUID means same logical node from client perspective; keep only the fastest one.
	slow.UUID = fast.UUID
	slow.StableID = slow.GenerateStableID()

	status := map[string]time.Duration{
		fast.StableID: 90 * time.Millisecond,
		slow.StableID: 250 * time.Millisecond,
		other.StableID: 120 * time.Millisecond,
	}

	got := selectTopBLByLatency([]*models.ProxyConfig{slow, fast, other}, func(stableID string) (bool, time.Duration, error) {
		return true, status[stableID], nil
	}, 10)

	if len(got.proxies) != 2 {
		t.Fatalf("expected 2 proxies after dedup, got %d", len(got.proxies))
	}
	if got.proxies[0].StableID != fast.StableID {
		t.Fatalf("expected fastest duplicate to be selected, got %s", got.proxies[0].Name)
	}
	if got.proxies[1].StableID != other.StableID {
		t.Fatalf("expected second proxy to be BL Other, got %s", got.proxies[1].Name)
	}
}

func TestAPITopBLSubscriptionHandlerToken(t *testing.T) {
	pc := checker.NewProxyChecker(nil, 10000, "http://127.0.0.1:1", 1, "http://example.com", "", 1, 1, "status", 1)
	handler := APITopBLSubscriptionHandler(pc, "super-secret-token")

	reqNoToken := httptest.NewRequest(http.MethodGet, "/api/v1/public/subscriptions/top-bl", nil)
	recNoToken := httptest.NewRecorder()
	handler.ServeHTTP(recNoToken, reqNoToken)
	if recNoToken.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without token, got %d", recNoToken.Code)
	}

	reqBadToken := httptest.NewRequest(http.MethodGet, "/api/v1/public/subscriptions/top-bl?token=bad", nil)
	recBadToken := httptest.NewRecorder()
	handler.ServeHTTP(recBadToken, reqBadToken)
	if recBadToken.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with bad token, got %d", recBadToken.Code)
	}

	reqGoodToken := httptest.NewRequest(http.MethodGet, "/api/v1/public/subscriptions/top-bl?token=super-secret-token", nil)
	recGoodToken := httptest.NewRecorder()
	handler.ServeHTTP(recGoodToken, reqGoodToken)
	if recGoodToken.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", recGoodToken.Code)
	}
}

func TestResolveSubscriptionLinksKeepsLastWhenAllNA(t *testing.T) {
	last := []string{"vless://old1", "vless://old2"}
	current := []string{"vless://new1"}
	selection := topSelectionResult{
		totalBL: 2,
		naCount: 2,
	}

	out, newLast := resolveSubscriptionLinks(current, selection, last)
	if len(out) != 2 || out[0] != "vless://old1" {
		t.Fatalf("expected output to keep last list, got: %v", out)
	}
	if len(newLast) != 2 || newLast[1] != "vless://old2" {
		t.Fatalf("expected cached last list unchanged, got: %v", newLast)
	}
}

func TestResolveSubscriptionLinksUpdatesLastOnFreshData(t *testing.T) {
	last := []string{"vless://old1"}
	current := []string{"vless://new1", "vless://new2"}
	selection := topSelectionResult{
		totalBL: 2,
		naCount: 0,
	}

	out, newLast := resolveSubscriptionLinks(current, selection, last)
	if len(out) != 2 || out[0] != "vless://new1" {
		t.Fatalf("expected output to use fresh list, got: %v", out)
	}
	if len(newLast) != 2 || newLast[1] != "vless://new2" {
		t.Fatalf("expected cached last list updated, got: %v", newLast)
	}
}

func newTestProxy(name, sourceLine string) *models.ProxyConfig {
	testProxySeq++
	p := &models.ProxyConfig{
		Protocol:   "vless",
		Server:     fmt.Sprintf("1.1.1.%d", testProxySeq),
		Port:       443,
		Name:       name,
		UUID:       fmt.Sprintf("11111111-1111-1111-1111-%012d", testProxySeq),
		SourceLine: sourceLine,
	}
	p.StableID = p.GenerateStableID()
	return p
}
