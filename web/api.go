package web

import (
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/logger"
	"xray-checker/models"
	"xray-checker/subscription"
)

//go:embed openapi.yaml
var openAPISpec []byte

type ProxyInfo struct {
	Index     int    `json:"index"`
	StableID  string `json:"stableId"`
	Name      string `json:"name"`
	SubName   string `json:"subName"`
	Server    string `json:"server"`
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	ProxyPort int    `json:"proxyPort"`
	Online    bool   `json:"online"`
	LatencyMs int64  `json:"latencyMs"`
	Config    string `json:"config,omitempty"`
}

type PublicProxyInfo struct {
	StableID  string `json:"stableId"`
	Name      string `json:"name"`
	Online    bool   `json:"online"`
	LatencyMs int64  `json:"latencyMs"`
}

type StatusResponse struct {
	Total        int   `json:"total"`
	Online       int   `json:"online"`
	Offline      int   `json:"offline"`
	AvgLatencyMs int64 `json:"avgLatencyMs"`
}

type ConfigResponse struct {
	CheckInterval              int      `json:"checkInterval"`
	CheckMethod                string   `json:"checkMethod"`
	Timeout                    int      `json:"timeout"`
	StartPort                  int      `json:"startPort"`
	SubscriptionUpdate         bool     `json:"subscriptionUpdate"`
	SubscriptionUpdateInterval int      `json:"subscriptionUpdateInterval"`
	SimulateLatency            bool     `json:"simulateLatency"`
	SubscriptionNames          []string `json:"subscriptionNames"`
}

type SystemInfoResponse struct {
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	UptimeSec int64  `json:"uptimeSec"`
	Instance  string `json:"instance"`
}

type SystemIPResponse struct {
	IP string `json:"ip"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type RemoteSourceInfo struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	FileName    string `json:"fileName"`
	LastChecked string `json:"lastChecked,omitempty"`
	LastUpdated string `json:"lastUpdated,omitempty"`
	Error       string `json:"error,omitempty"`
}

type RemoteStateResponse struct {
	IntervalSeconds int                `json:"intervalSeconds"`
	DownloadDir     string             `json:"downloadDir"`
	Sources         []RemoteSourceInfo `json:"sources"`
}

type rankedProxy struct {
	proxy   *models.ProxyConfig
	latency time.Duration
	key     string
}

type keyStatusCounts struct {
	online  int
	offline int
	na      int
}

type topSelectionResult struct {
	proxies      []rankedProxy
	totalBL      int
	naCount      int
	onlineCount  int
	offlineCount int
	keyStates    map[string]keyStatusCounts
}

type activeEntry struct {
	item      rankedProxy
	addedAt   time.Time
	badStreak int
}

type stableTopBLSelector struct {
	limit         int
	mu            sync.Mutex
	emaByKey      map[string]time.Duration
	active        map[string]*activeEntry
	published     []string
	lastPublished time.Time
	hadEmergency  bool
}

const (
	topBLBatchInterval  = 2 * time.Hour
	topBLMinHold        = 2 * time.Hour
	topBLEMAAlpha       = 0.3
	topBLReplaceMinMs   = 50 * time.Millisecond
	topBLReplaceMinGain = 0.20
	topBLBadStreakLimit = 2
	topBLQuota          = 10
	topCIDRQuota        = 10
)

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}

func toProxyInfo(proxy *models.ProxyConfig, online bool, latency time.Duration, startPort int) ProxyInfo {
	return ProxyInfo{
		Index:     proxy.Index,
		StableID:  proxy.StableID,
		Name:      sanitizeText(proxy.Name),
		SubName:   proxy.SubName,
		Server:    sanitizeText(proxy.Server),
		Port:      proxy.Port,
		Protocol:  proxy.Protocol,
		ProxyPort: startPort + proxy.Index,
		Online:    online,
		LatencyMs: latency.Milliseconds(),
		Config:    sanitizeConfig(proxy.SourceLine),
	}
}

// APIPublicProxiesHandler returns public info for all proxies (no auth required)
// @Summary List all proxies (public)
// @Description Returns a list of all proxies with status (no sensitive data, no auth)
// @Tags public
// @Produce json
// @Success 200 {array} PublicProxyInfo
// @Router /api/v1/public/proxies [get]
func APIPublicProxiesHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxies := proxyChecker.GetProxies()
		logger.Debug("API public proxies requested: %d", len(proxies))
		result := make([]PublicProxyInfo, 0, len(proxies))

		for _, proxy := range proxies {
			status, latency, _ := proxyChecker.GetProxyStatusByStableID(proxy.StableID)
			result = append(result, PublicProxyInfo{
				StableID:  proxy.StableID,
				Name:      sanitizeText(proxy.Name),
				Online:    status,
				LatencyMs: latency.Milliseconds(),
			})
		}

		writeJSON(w, result)
	}
}

// APIProxiesHandler returns info for all proxies
// @Summary List all proxies
// @Description Returns a list of all proxies with status information
// @Tags proxies
// @Produce json
// @Success 200 {array} ProxyInfo
// @Router /api/v1/proxies [get]
func APIProxiesHandler(proxyChecker *checker.ProxyChecker, startPort int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxies := proxyChecker.GetProxies()
		logger.Debug("API proxies requested: %d", len(proxies))
		result := make([]ProxyInfo, 0, len(proxies))

		for _, proxy := range proxies {
			status, latency, _ := proxyChecker.GetProxyStatusByStableID(proxy.StableID)
			result = append(result, toProxyInfo(proxy, status, latency, startPort))
		}

		writeJSON(w, result)
	}
}

// APIProxyHandler returns info for a single proxy
// @Summary Get proxy by ID
// @Description Returns information for a specific proxy
// @Tags proxies
// @Produce json
// @Param stableID path string true "Proxy Stable ID"
// @Success 200 {object} ProxyInfo
// @Failure 404 {object} map[string]string
// @Router /api/v1/proxies/{stableID} [get]
func APIProxyHandler(proxyChecker *checker.ProxyChecker, startPort int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		prefix := "/api/v1/proxies/"
		if !strings.HasPrefix(path, prefix) {
			writeError(w, "Invalid path", http.StatusBadRequest)
			return
		}

		stableID := strings.TrimPrefix(path, prefix)
		if stableID == "" {
			writeError(w, "Proxy ID is required", http.StatusBadRequest)
			return
		}

		proxy, exists := proxyChecker.GetProxyByStableID(stableID)
		if !exists {
			writeError(w, "Proxy not found", http.StatusNotFound)
			return
		}

		status, latency, _ := proxyChecker.GetProxyStatusByStableID(proxy.StableID)
		writeJSON(w, toProxyInfo(proxy, status, latency, startPort))
	}
}

// APIStatusHandler returns system status summary
// @Summary Get system status
// @Description Returns summary statistics about all proxies
// @Tags status
// @Produce json
// @Success 200 {object} StatusResponse
// @Router /api/v1/status [get]
func APIStatusHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxies := proxyChecker.GetProxies()

		var online, offline int
		var totalLatency int64
		var latencyCount int

		for _, proxy := range proxies {
			status, latency, _ := proxyChecker.GetProxyStatusByStableID(proxy.StableID)
			if status {
				online++
				if latency > 0 {
					totalLatency += latency.Milliseconds()
					latencyCount++
				}
			} else {
				offline++
			}
		}

		var avgLatency int64
		if latencyCount > 0 {
			avgLatency = totalLatency / int64(latencyCount)
		}

		writeJSON(w, StatusResponse{
			Total:        len(proxies),
			Online:       online,
			Offline:      offline,
			AvgLatencyMs: avgLatency,
		})
	}
}

// APIConfigHandler returns current configuration
// @Summary Get current configuration
// @Description Returns the current checker configuration
// @Tags config
// @Produce json
// @Success 200 {object} ConfigResponse
// @Router /api/v1/config [get]
func APIConfigHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subNames := CollectSubscriptionNames(proxyChecker.GetProxies())
		writeJSON(w, ConfigResponse{
			CheckInterval:              config.CLIConfig.Proxy.CheckInterval,
			CheckMethod:                config.CLIConfig.Proxy.CheckMethod,
			Timeout:                    config.CLIConfig.Proxy.Timeout,
			StartPort:                  config.CLIConfig.Xray.StartPort,
			SubscriptionUpdate:         config.CLIConfig.Subscription.Update,
			SubscriptionUpdateInterval: config.CLIConfig.Subscription.UpdateInterval,
			SimulateLatency:            config.CLIConfig.Proxy.SimulateLatency,
			SubscriptionNames:          subNames,
		})
	}
}

func CollectSubscriptionNames(proxies []*models.ProxyConfig) []string {
	seen := make(map[string]bool)
	var names []string
	for _, proxy := range proxies {
		if proxy.SubName != "" && !seen[proxy.SubName] {
			seen[proxy.SubName] = true
			names = append(names, proxy.SubName)
		}
	}
	return names
}

// APISystemInfoHandler returns system info
// @Summary Get system info
// @Description Returns version, uptime, and instance information
// @Tags system
// @Produce json
// @Success 200 {object} SystemInfoResponse
// @Router /api/v1/system/info [get]
func APISystemInfoHandler(version string, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime)
		writeJSON(w, SystemInfoResponse{
			Version:   version,
			Uptime:    formatDuration(uptime),
			UptimeSec: int64(uptime.Seconds()),
			Instance:  config.CLIConfig.Metrics.Instance,
		})
	}
}

// APISystemIPHandler returns current IP
// @Summary Get current IP
// @Description Returns the current detected IP address
// @Tags system
// @Produce json
// @Success 200 {object} SystemIPResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/system/ip [get]
func APISystemIPHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, err := proxyChecker.GetCurrentIP()
		if err != nil {
			writeError(w, "Failed to get IP", http.StatusInternalServerError)
			return
		}
		writeJSON(w, SystemIPResponse{IP: ip})
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func APIOpenAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(openAPISpec)
	}
}

func APIDocsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(swaggerUIHTML))
	}
}

// APITopBLSubscriptionHandler returns base64-encoded subscription with top fastest BL and CIDR configs.
func APITopBLSubscriptionHandler(proxyChecker *checker.ProxyChecker, requiredToken string) http.HandlerFunc {
	selector := newStableTopBLSelector(topBLQuota + topCIDRQuota)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if strings.TrimSpace(requiredToken) != "" {
			providedToken := r.URL.Query().Get("token")
			if !secureTokenEquals(providedToken, requiredToken) {
				http.NotFound(w, r)
				return
			}
		}

		links := selector.Next(proxyChecker.GetProxies(), proxyChecker.GetProxyStatusByStableID, time.Now())

		payload := strings.Join(links, "\n")
		encoded := base64.StdEncoding.EncodeToString([]byte(payload))

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("X-Subscription-Configs", fmt.Sprintf("%d", len(links)))
		_, _ = w.Write([]byte(encoded))
	}
}

func secureTokenEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func newStableTopBLSelector(limit int) *stableTopBLSelector {
	if limit <= 0 {
		limit = 10
	}
	return &stableTopBLSelector{
		limit:    limit,
		emaByKey: make(map[string]time.Duration),
		active:   make(map[string]*activeEntry),
	}
}

func (s *stableTopBLSelector) Next(
	proxies []*models.ProxyConfig,
	statusFn func(string) (bool, time.Duration, error),
	now time.Time,
) []string {
	selection := selectTopBLAndCIDRByLatency(proxies, statusFn, topBLQuota, topCIDRQuota)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Keep previous published list when all BL metrics are n/a.
	if selection.totalBL > 0 && selection.naCount == selection.totalBL && len(s.published) > 0 {
		return append([]string(nil), s.published...)
	}

	ranked := s.applyEMA(selection.proxies)
	s.reconcileActive(ranked, selection.keyStates, now)

	activeRanked := s.activeRanked()
	proposedLinks := linksFromRanked(activeRanked)
	if len(proposedLinks) == 0 && len(s.published) > 0 {
		return append([]string(nil), s.published...)
	}

	emergencyChange := s.hadEmergency
	s.hadEmergency = false
	shouldPublish := len(s.published) == 0 ||
		now.Sub(s.lastPublished) >= topBLBatchInterval ||
		emergencyChange

	if shouldPublish && len(proposedLinks) > 0 {
		s.published = append(s.published[:0], proposedLinks...)
		s.lastPublished = now
	}

	return append([]string(nil), s.published...)
}

func selectTopBLAndCIDRByLatency(
	proxies []*models.ProxyConfig,
	statusFn func(string) (bool, time.Duration, error),
	blLimit int,
	cidrLimit int,
) topSelectionResult {
	if blLimit < 0 {
		blLimit = 0
	}
	if cidrLimit < 0 {
		cidrLimit = 0
	}

	result := topSelectionResult{
		keyStates: make(map[string]keyStatusCounts),
	}
	uniqueByKey := make(map[string]rankedProxy, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil || strings.TrimSpace(proxy.SourceLine) == "" {
			continue
		}

		nameUpper := strings.ToUpper(proxy.Name)
		hasBL := strings.Contains(nameUpper, "BL")
		hasCIDR := strings.Contains(nameUpper, "CIDR")
		if !hasBL && !hasCIDR {
			continue
		}

		result.totalBL++
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}
		key := dedupKey(proxy)

		online, latency, err := statusFn(proxy.StableID)
		if err != nil {
			result.naCount++
			st := result.keyStates[key]
			st.na++
			result.keyStates[key] = st
			continue
		}
		if !online {
			result.offlineCount++
			st := result.keyStates[key]
			st.offline++
			result.keyStates[key] = st
			continue
		}
		result.onlineCount++
		st := result.keyStates[key]
		st.online++
		result.keyStates[key] = st

		candidate := rankedProxy{
			proxy:   proxy,
			latency: latency,
			key:     key,
		}
		if existing, ok := uniqueByKey[key]; ok {
			if isBetterCandidate(candidate, existing) {
				uniqueByKey[key] = candidate
			}
			continue
		}
		uniqueByKey[key] = candidate
	}

	ranked := make([]rankedProxy, 0, len(uniqueByKey))
	for _, item := range uniqueByKey {
		ranked = append(ranked, item)
	}
	sort.Slice(ranked, func(i, j int) bool { return isBetterCandidate(ranked[i], ranked[j]) })

	selected := make([]rankedProxy, 0, blLimit+cidrLimit)
	selectedByKey := make(map[string]struct{}, blLimit+cidrLimit)
	blCount, cidrCount := 0, 0

	for _, item := range ranked {
		if blCount >= blLimit && cidrCount >= cidrLimit {
			break
		}
		if _, exists := selectedByKey[item.key]; exists {
			continue
		}

		nameUpper := strings.ToUpper(item.proxy.Name)
		hasBL := strings.Contains(nameUpper, "BL")
		hasCIDR := strings.Contains(nameUpper, "CIDR")
		if !hasBL && !hasCIDR {
			continue
		}

		switch {
		case hasBL && hasCIDR:
			blNeed := blLimit - blCount
			cidrNeed := cidrLimit - cidrCount
			if blNeed <= 0 && cidrNeed <= 0 {
				continue
			}
			// Prefer filling the most-empty bucket first.
			if cidrNeed > blNeed {
				cidrCount++
			} else {
				blCount++
			}
		case hasBL:
			if blCount >= blLimit {
				continue
			}
			blCount++
		case hasCIDR:
			if cidrCount >= cidrLimit {
				continue
			}
			cidrCount++
		}

		selectedByKey[item.key] = struct{}{}
		selected = append(selected, item)
	}

	result.proxies = selected
	return result
}

func (s *stableTopBLSelector) applyEMA(proxies []rankedProxy) []rankedProxy {
	ranked := make([]rankedProxy, 0, len(proxies))
	for _, p := range proxies {
		key := p.key
		rawMs := p.latency
		prev, ok := s.emaByKey[key]
		var ema time.Duration
		if !ok || prev <= 0 {
			ema = rawMs
		} else {
			ema = time.Duration((1.0-topBLEMAAlpha)*float64(prev) + topBLEMAAlpha*float64(rawMs))
		}
		s.emaByKey[key] = ema

		ranked = append(ranked, rankedProxy{
			proxy:   p.proxy,
			latency: ema,
			key:     key,
		})
	}

	sort.Slice(ranked, func(i, j int) bool { return isBetterCandidate(ranked[i], ranked[j]) })
	return ranked
}

func (s *stableTopBLSelector) reconcileActive(ranked []rankedProxy, keyStates map[string]keyStatusCounts, now time.Time) {
	byKey := make(map[string]rankedProxy, len(ranked))
	for _, r := range ranked {
		if _, exists := byKey[r.key]; !exists {
			byKey[r.key] = r
		}
	}

	for key, entry := range s.active {
		st := keyStates[key]
		if st.online > 0 {
			entry.badStreak = 0
		} else if st.offline > 0 || st.na > 0 {
			entry.badStreak++
		} else {
			entry.badStreak++
		}
		if candidate, ok := byKey[key]; ok {
			entry.item = candidate
		}
		if entry.badStreak >= topBLBadStreakLimit {
			delete(s.active, key)
			s.hadEmergency = true
		}
	}

	for _, c := range ranked {
		if len(s.active) >= s.limit {
			break
		}
		if _, exists := s.active[c.key]; exists {
			continue
		}
		s.active[c.key] = &activeEntry{item: c, addedAt: now}
	}

	for _, c := range ranked {
		if _, exists := s.active[c.key]; exists {
			continue
		}
		worstKey, worstEntry := s.findWorstReplaceable(now)
		if worstEntry == nil {
			break
		}
		if !isSignificantImprovement(c.latency, worstEntry.item.latency) {
			continue
		}
		delete(s.active, worstKey)
		s.active[c.key] = &activeEntry{item: c, addedAt: now}
	}
}

func (s *stableTopBLSelector) findWorstReplaceable(now time.Time) (string, *activeEntry) {
	var worstKey string
	var worstEntry *activeEntry
	for key, entry := range s.active {
		holdPassed := now.Sub(entry.addedAt) >= topBLMinHold
		if !holdPassed && entry.badStreak < topBLBadStreakLimit {
			continue
		}
		if worstEntry == nil || isBetterCandidate(worstEntry.item, entry.item) {
			worstKey = key
			worstEntry = entry
		}
	}
	return worstKey, worstEntry
}

func isSignificantImprovement(candidate, current time.Duration) bool {
	if candidate >= current {
		return false
	}
	if current-candidate >= topBLReplaceMinMs {
		return true
	}
	if current <= 0 {
		return false
	}
	ratioGain := float64(current-candidate) / float64(current)
	return ratioGain >= topBLReplaceMinGain
}

func (s *stableTopBLSelector) activeRanked() []rankedProxy {
	ranked := make([]rankedProxy, 0, len(s.active))
	for _, entry := range s.active {
		ranked = append(ranked, entry.item)
	}
	sort.Slice(ranked, func(i, j int) bool { return isBetterCandidate(ranked[i], ranked[j]) })
	if len(ranked) > s.limit {
		ranked = ranked[:s.limit]
	}
	return ranked
}

func linksFromRanked(ranked []rankedProxy) []string {
	links := make([]string, 0, len(ranked))
	for _, item := range ranked {
		line := sanitizeConfig(item.proxy.SourceLine)
		if line == "" {
			continue
		}
		links = append(links, line)
	}
	return links
}

func selectTopBLByLatency(
	proxies []*models.ProxyConfig,
	statusFn func(string) (bool, time.Duration, error),
	limit int,
) topSelectionResult {
	if limit <= 0 {
		limit = 10
	}

	result := topSelectionResult{
		keyStates: make(map[string]keyStatusCounts),
	}
	uniqueByKey := make(map[string]rankedProxy, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil || strings.TrimSpace(proxy.SourceLine) == "" {
			continue
		}
		if !strings.Contains(strings.ToUpper(proxy.Name), "BL") {
			continue
		}
		result.totalBL++
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}
		key := dedupKey(proxy)

		online, latency, err := statusFn(proxy.StableID)
		if err != nil {
			result.naCount++
			st := result.keyStates[key]
			st.na++
			result.keyStates[key] = st
			continue
		}
		if !online {
			result.offlineCount++
			st := result.keyStates[key]
			st.offline++
			result.keyStates[key] = st
			continue
		}
		result.onlineCount++
		st := result.keyStates[key]
		st.online++
		result.keyStates[key] = st

		candidate := rankedProxy{
			proxy:   proxy,
			latency: latency,
			key:     key,
		}

		if existing, ok := uniqueByKey[key]; ok {
			if isBetterCandidate(candidate, existing) {
				uniqueByKey[key] = candidate
			}
			continue
		}
		uniqueByKey[key] = candidate
	}

	ranked := make([]rankedProxy, 0, len(uniqueByKey))
	for _, item := range uniqueByKey {
		ranked = append(ranked, item)
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].latency != ranked[j].latency {
			return ranked[i].latency < ranked[j].latency
		}

		leftName := strings.ToLower(strings.TrimSpace(ranked[i].proxy.Name))
		rightName := strings.ToLower(strings.TrimSpace(ranked[j].proxy.Name))
		if leftName != rightName {
			return leftName < rightName
		}

		return ranked[i].proxy.StableID < ranked[j].proxy.StableID
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	result.proxies = ranked
	return result
}

func dedupKey(proxy *models.ProxyConfig) string {
	protocol := strings.ToLower(strings.TrimSpace(proxy.Protocol))
	if sid := strings.TrimSpace(proxy.StableID); sid != "" {
		return protocol + "|stable|" + sid
	}
	if id := strings.TrimSpace(proxy.UUID); id != "" {
		return protocol + "|uuid|" + id
	}
	if pw := strings.TrimSpace(proxy.Password); pw != "" {
		return protocol + "|password|" + pw
	}
	return protocol + "|name|" + strings.ToLower(strings.TrimSpace(proxy.Name))
}

func isBetterCandidate(left, right rankedProxy) bool {
	if left.latency != right.latency {
		return left.latency < right.latency
	}

	leftName := strings.ToLower(strings.TrimSpace(left.proxy.Name))
	rightName := strings.ToLower(strings.TrimSpace(right.proxy.Name))
	if leftName != rightName {
		return leftName < rightName
	}
	return left.proxy.StableID < right.proxy.StableID
}

func APIRemoteSourcesHandler(manager *subscription.RemoteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, "Remote subscriptions not configured", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			state := manager.GetState()
			resp := RemoteStateResponse{
				IntervalSeconds: state.IntervalSeconds,
				DownloadDir:     manager.DownloadDir(),
				Sources:         make([]RemoteSourceInfo, 0, len(state.Sources)),
			}
			for _, src := range state.Sources {
				resp.Sources = append(resp.Sources, RemoteSourceInfo{
					ID:          src.ID,
					URL:         src.URL,
					FileName:    src.FileName,
					LastChecked: formatTime(src.LastChecked),
					LastUpdated: formatTime(src.LastUpdated),
					Error:       src.Error,
				})
			}
			writeJSON(w, resp)
			return
		case http.MethodPost:
			var req struct {
				URLs []string `json:"urls"`
				URL  string   `json:"url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			if req.URL != "" {
				req.URLs = append(req.URLs, req.URL)
			}
			if len(req.URLs) == 0 {
				writeError(w, "No URLs provided", http.StatusBadRequest)
				return
			}
			added, err := manager.AddURLs(req.URLs)
			if err != nil {
				writeError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, added)
			return
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				id = r.URL.Query().Get("url")
			}
			if id == "" {
				writeError(w, "id or url is required", http.StatusBadRequest)
				return
			}
			if !manager.RemoveByID(id) {
				writeError(w, "source not found", http.StatusNotFound)
				return
			}
			writeJSON(w, map[string]string{"status": "removed"})
			return
		default:
			writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func APIRemoteIntervalHandler(manager *subscription.RemoteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, "Remote subscriptions not configured", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPut {
			writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			IntervalSeconds int `json:"intervalSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.IntervalSeconds <= 0 {
			writeError(w, "Interval must be greater than 0", http.StatusBadRequest)
			return
		}
		manager.SetInterval(req.IntervalSeconds)
		writeJSON(w, map[string]int{"intervalSeconds": req.IntervalSeconds})
	}
}

func APIRemoteRefreshHandler(manager *subscription.RemoteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil {
			writeError(w, "Remote subscriptions not configured", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost {
			writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		updated, err := manager.CheckUpdates()
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]int{"updated": updated})
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Xray Checker API</title>
  <style>
    body { margin: 0; padding: 0; }
    .swagger-ui .topbar { display: none; }
  </style>
  <script>
    // Detect base path from current URL (e.g., /xray/api/v1/docs -> /xray)
    (function() {
      const path = window.location.pathname;
      const apiIdx = path.indexOf('/api/v1/docs');
      const basePath = apiIdx > 0 ? path.substring(0, apiIdx) : '';
      document.write('<link rel="stylesheet" href="' + basePath + '/static/swagger-ui.css">');
    })();
  </script>
</head>
<body>
  <div id="swagger-ui"></div>
  <script>
    (function() {
      const path = window.location.pathname;
      const apiIdx = path.indexOf('/api/v1/docs');
      const basePath = apiIdx > 0 ? path.substring(0, apiIdx) : '';

      const script = document.createElement('script');
      script.src = basePath + '/static/swagger-ui-bundle.js';
      script.onload = function() {
        SwaggerUIBundle({
          url: './openapi.yaml',
          dom_id: '#swagger-ui',
          presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
          layout: 'BaseLayout'
        });
      };
      document.body.appendChild(script);
    })();
  </script>
</body>
</html>`
