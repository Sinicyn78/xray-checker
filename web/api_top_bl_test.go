package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
		proxies[2].StableID: {online: true, latency: 10 * time.Millisecond},  // no BL in name
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

	if got.proxies[0].proxy.Name != "bl Beta" {
		t.Fatalf("expected fastest BL proxy to be bl Beta, got %s", got.proxies[0].proxy.Name)
	}
	if got.proxies[1].proxy.Name != "BL Alpha" {
		t.Fatalf("expected second BL proxy to be BL Alpha, got %s", got.proxies[1].proxy.Name)
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

	if got.proxies[0].proxy.Name != "BL Node 00" {
		t.Fatalf("expected first proxy BL Node 00, got %s", got.proxies[0].proxy.Name)
	}
	if got.proxies[9].proxy.Name != "BL Node 09" {
		t.Fatalf("expected tenth proxy BL Node 09, got %s", got.proxies[9].proxy.Name)
	}
}

func TestSelectTopBLByLatencyDeduplicatesByStableID(t *testing.T) {
	base := newTestProxy("BL Base", "vless://base")
	duplicate := &models.ProxyConfig{
		Protocol:   base.Protocol,
		Server:     base.Server,
		Port:       base.Port,
		Name:       "BL Duplicate",
		UUID:       base.UUID,
		Security:   base.Security,
		Type:       base.Type,
		SNI:        base.SNI,
		SourceLine: "vless://duplicate",
	}
	duplicate.StableID = duplicate.GenerateStableID()
	other := newTestProxy("BL Other", "vless://other")

	status := map[string]time.Duration{
		base.StableID:  90 * time.Millisecond,
		other.StableID: 120 * time.Millisecond,
	}

	got := selectTopBLByLatency([]*models.ProxyConfig{base, duplicate, other}, func(stableID string) (bool, time.Duration, error) {
		return true, status[stableID], nil
	}, 10)

	if len(got.proxies) != 2 {
		t.Fatalf("expected 2 proxies after dedup, got %d", len(got.proxies))
	}
	if got.proxies[0].proxy.StableID != base.StableID {
		t.Fatalf("expected one stable duplicate to remain, got %s", got.proxies[0].proxy.Name)
	}
	if got.proxies[1].proxy.StableID != other.StableID {
		t.Fatalf("expected second proxy to be BL Other, got %s", got.proxies[1].proxy.Name)
	}
}

func TestSelectTopBLAndCIDRByLatencyQuotas(t *testing.T) {
	proxies := make([]*models.ProxyConfig, 0, 24)
	latencyByID := make(map[string]time.Duration, 24)

	for i := 0; i < 12; i++ {
		p := newTestProxy(fmt.Sprintf("BL Node %02d", i), fmt.Sprintf("vless://bl-%d", i))
		proxies = append(proxies, p)
		latencyByID[p.StableID] = time.Duration(10+i) * time.Millisecond
	}
	for i := 0; i < 12; i++ {
		p := newTestProxy(fmt.Sprintf("CIDR Node %02d", i), fmt.Sprintf("vless://cidr-%d", i))
		proxies = append(proxies, p)
		latencyByID[p.StableID] = time.Duration(10+i) * time.Millisecond
	}

	got := selectTopBLAndCIDRByLatency(proxies, func(stableID string) (bool, time.Duration, error) {
		return true, latencyByID[stableID], nil
	}, 10, 10)

	if len(got.proxies) != 20 {
		t.Fatalf("expected 20 proxies total, got %d", len(got.proxies))
	}

	blCount := 0
	cidrCount := 0
	for _, rp := range got.proxies {
		name := strings.ToUpper(rp.proxy.Name)
		if strings.Contains(name, "BL") {
			blCount++
		}
		if strings.Contains(name, "CIDR") {
			cidrCount++
		}
	}
	if blCount != 10 {
		t.Fatalf("expected 10 BL proxies, got %d", blCount)
	}
	if cidrCount != 10 {
		t.Fatalf("expected 10 CIDR proxies, got %d", cidrCount)
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

func TestStableTopBLSelectorKeepsPublishedWhenAllNA(t *testing.T) {
	selector := newStableTopBLSelector(10)
	now := time.Now()

	p1 := newTestProxy("BL One", "vless://one")
	p2 := newTestProxy("BL Two", "vless://two")
	proxies := []*models.ProxyConfig{p1, p2}

	statusOK := map[string]time.Duration{
		p1.StableID: 100 * time.Millisecond,
		p2.StableID: 120 * time.Millisecond,
	}
	first := selector.Next(proxies, func(stableID string) (bool, time.Duration, error) {
		return true, statusOK[stableID], nil
	}, now)
	if len(first) != 2 {
		t.Fatalf("expected first publish of 2 links, got %d", len(first))
	}

	second := selector.Next(proxies, func(stableID string) (bool, time.Duration, error) {
		return false, 0, fmt.Errorf("n/a")
	}, now.Add(5*time.Minute))
	if len(second) != 2 || second[0] != first[0] {
		t.Fatalf("expected published links to be preserved on all n/a, got %v", second)
	}
}

func TestStableTopBLSelectorHysteresisAndHold(t *testing.T) {
	selector := newStableTopBLSelector(1)
	now := time.Now()

	incumbent := newTestProxy("BL Incumbent", "vless://incumbent")
	challenger := newTestProxy("BL Challenger", "vless://challenger")

	// Initial publish with incumbent.
	out1 := selector.Next([]*models.ProxyConfig{incumbent}, func(stableID string) (bool, time.Duration, error) {
		return true, 200 * time.Millisecond, nil
	}, now)
	if len(out1) != 1 || out1[0] != sanitizeConfig(incumbent.SourceLine) {
		t.Fatalf("unexpected initial output: %v", out1)
	}

	// Challenger is only 20ms faster and within hold interval: should not replace.
	out2 := selector.Next([]*models.ProxyConfig{incumbent, challenger}, func(stableID string) (bool, time.Duration, error) {
		if stableID == incumbent.StableID {
			return true, 200 * time.Millisecond, nil
		}
		return true, 180 * time.Millisecond, nil
	}, now.Add(10*time.Minute))
	if out2[0] != sanitizeConfig(incumbent.SourceLine) {
		t.Fatalf("expected incumbent to stay during hold/hysteresis, got %v", out2)
	}

	// After hold, challenger is significantly faster: replacement allowed.
	out3 := selector.Next([]*models.ProxyConfig{incumbent, challenger}, func(stableID string) (bool, time.Duration, error) {
		if stableID == incumbent.StableID {
			return true, 250 * time.Millisecond, nil
		}
		return true, 120 * time.Millisecond, nil
	}, now.Add(3*time.Hour))
	if out3[0] != sanitizeConfig(challenger.SourceLine) {
		t.Fatalf("expected challenger to replace incumbent after hold with significant gain, got %v", out3)
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
