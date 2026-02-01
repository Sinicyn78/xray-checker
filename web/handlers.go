package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"
	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/metrics"
	"xray-checker/models"
	"xray-checker/subscription"
)

var (
	registeredEndpoints []EndpointInfo
	endpointsMu         sync.RWMutex
)

type EndpointInfo struct {
	Name       string
	ServerInfo string
	URL        string
	ProxyPort  int
	Index      int
	Status     bool
	Latency    time.Duration
	StableID   string
	Config     string
}

func IndexHandler(version string, proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		RegisterConfigEndpoints(proxyChecker.GetProxies(), proxyChecker, config.CLIConfig.Xray.StartPort)

		endpointsMu.RLock()
		allEndpoints := make([]EndpointInfo, len(registeredEndpoints))
		copy(allEndpoints, registeredEndpoints)
		endpointsMu.RUnlock()

		isPublic := config.CLIConfig.Web.Public
		showServerDetails := config.CLIConfig.Web.ShowServerDetails
		if isPublic {
			showServerDetails = false
		}

		endpoints := allEndpoints
		if isPublic {
			endpoints = make([]EndpointInfo, len(allEndpoints))
			for i, ep := range allEndpoints {
				endpoints[i] = EndpointInfo{
					Name:     ep.Name,
					Index:    ep.Index,
					Status:   ep.Status,
					Latency:  ep.Latency,
					StableID: ep.StableID,
				}
			}
		}

		endpointsJSON := buildEndpointsJSON(endpoints, showServerDetails, isPublic)

		data := PageData{
			Version:                    version,
			Host:                       config.CLIConfig.Metrics.Host,
			Port:                       config.CLIConfig.Metrics.Port,
			CheckInterval:              config.CLIConfig.Proxy.CheckInterval,
			IPCheckUrl:                 config.CLIConfig.Proxy.IpCheckUrl,
			CheckMethod:                config.CLIConfig.Proxy.CheckMethod,
			StatusCheckUrl:             config.CLIConfig.Proxy.StatusCheckUrl,
			DownloadUrl:                config.CLIConfig.Proxy.DownloadUrl,
			SimulateLatency:            config.CLIConfig.Proxy.SimulateLatency,
			Timeout:                    config.CLIConfig.Proxy.Timeout,
			SubscriptionUpdate:         config.CLIConfig.Subscription.Update,
			SubscriptionUpdateInterval: config.CLIConfig.Subscription.UpdateInterval,
			StartPort:                  config.CLIConfig.Xray.StartPort,
			Instance:                   config.CLIConfig.Metrics.Instance,
			PushUrl:                    metrics.GetPushURL(config.CLIConfig.Metrics.PushURL),
			Endpoints:                  endpoints,
			EndpointsJSON:              endpointsJSON,
			ShowServerDetails:          showServerDetails,
			IsPublic:                   isPublic,
			SubscriptionName:           subscription.GetSubscriptionName(),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		if err := RenderIndex(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

type endpointView struct {
	Name       string `json:"name"`
	StableID   string `json:"stableId"`
	Status     bool   `json:"status"`
	Latency    string `json:"latency"`
	LatencyMs  int64  `json:"latencyMs"`
	Index      int    `json:"index"`
	URL        string `json:"url,omitempty"`
	ServerInfo string `json:"serverInfo,omitempty"`
	ProxyPort  int    `json:"proxyPort,omitempty"`
	Config     string `json:"config,omitempty"`
}

func buildEndpointsJSON(endpoints []EndpointInfo, showServerDetails bool, isPublic bool) template.JS {
	view := make([]endpointView, 0, len(endpoints))
	for _, ep := range endpoints {
		latency := "n/a"
		if ep.Latency > 0 {
			latency = fmt.Sprintf("%dms", ep.Latency.Milliseconds())
		}
		item := endpointView{
			Name:      sanitizeText(ep.Name),
			StableID:  ep.StableID,
			Status:    ep.Status,
			Latency:   latency,
			LatencyMs: ep.Latency.Milliseconds(),
			Index:     ep.Index,
		}
		if !isPublic {
			item.URL = ep.URL
			item.Config = sanitizeConfig(ep.Config)
		}
		if showServerDetails {
			item.ServerInfo = sanitizeText(ep.ServerInfo)
			item.ProxyPort = ep.ProxyPort
		}
		view = append(view, item)
	}

	data, err := json.Marshal(view)
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(data)
}

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

func BasicAuthMiddleware(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || user != username || pass != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
				http.Error(w, "Unauthorized.", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ConfigStatusHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/config/"):]
		if path == "" {
			http.Error(w, "Config path is required", http.StatusBadRequest)
			return
		}

		found, exists := proxyChecker.GetProxyByStableID(path)
		if !exists {
			http.Error(w, "Config not found", http.StatusNotFound)
			return
		}

		status, latency, err := proxyChecker.GetProxyStatus(found.Name)
		if err != nil {
			http.Error(w, "Status not available", http.StatusNotFound)
			return
		}

		if config.CLIConfig.Proxy.SimulateLatency {
			time.Sleep(time.Duration(latency))
		}

		if status {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Failed"))
		}
	}
}

func RegisterConfigEndpoints(proxies []*models.ProxyConfig, proxyChecker *checker.ProxyChecker, startPort int) {
	endpoints := make([]EndpointInfo, 0, len(proxies))

	for _, proxy := range proxies {
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}

		endpoint := fmt.Sprintf("./config/%s", proxy.StableID)
		displayName := sanitizeText(proxy.Name)

		status, latency, _ := proxyChecker.GetProxyStatus(proxy.Name)

		endpoints = append(endpoints, EndpointInfo{
			Name:       displayName,
			ServerInfo: sanitizeText(fmt.Sprintf("%s:%d", proxy.Server, proxy.Port)),
			URL:        endpoint,
			ProxyPort:  startPort + proxy.Index,
			Index:      proxy.Index,
			Status:     status,
			Latency:    latency,
			StableID:   proxy.StableID,
			Config:     proxy.SourceLine,
		})
	}

	endpointsMu.Lock()
	registeredEndpoints = endpoints
	endpointsMu.Unlock()
}

type PrefixServeMux struct {
	prefix string
	mux    *http.ServeMux
}

func NewPrefixServeMux(prefix string) (*PrefixServeMux, error) {
	if strings.HasSuffix(prefix, "/") {
		return nil, fmt.Errorf("served url path prefix '%s' should not ends with a '/'", prefix)
	}
	return &PrefixServeMux{
		prefix: prefix,
		mux:    http.NewServeMux(),
	}, nil
}

func (pm *PrefixServeMux) Handle(pattern string, handler http.Handler) {
	pm.mux.Handle(pm.prefix+pattern, http.StripPrefix(pm.prefix, handler))
}

func (pm *PrefixServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == pm.prefix || strings.HasPrefix(r.URL.Path, pm.prefix+"/") {
		pm.mux.ServeHTTP(w, r)
	} else {
		http.NotFound(w, r)
	}
}
