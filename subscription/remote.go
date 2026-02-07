package subscription

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"xray-checker/config"
	"xray-checker/logger"
)

type RemoteSource struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	LastChecked  time.Time `json:"lastChecked,omitempty"`
	LastUpdated  time.Time `json:"lastUpdated,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type RemoteState struct {
	IntervalSeconds int            `json:"intervalSeconds"`
	Sources         []RemoteSource `json:"sources"`
}

type RemoteManager struct {
	mu          sync.Mutex
	state       RemoteState
	statePath   string
	downloadDir string
	client      *http.Client
}

var (
	remoteOnce     sync.Once
	remoteInstance *RemoteManager
	remoteErr      error
)

func GetDownloadDirectory() (string, error) {
	for _, src := range config.CLIConfig.Subscription.URLs {
		if !strings.HasPrefix(src, "file://") {
			continue
		}
		path := strings.TrimPrefix(src, "file://")
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return path, nil
			}
			return filepath.Dir(path), nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".txt" || ext == ".json" {
			return filepath.Dir(path), nil
		}
		return path, nil
	}
	return "", fmt.Errorf("no file:// subscription URL configured")
}

func GetRemoteManager() (*RemoteManager, error) {
	remoteOnce.Do(func() {
		dir, err := GetDownloadDirectory()
		if err != nil {
			remoteErr = err
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			remoteErr = err
			return
		}
		statePath := remoteStatePath(dir)
		if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
			remoteErr = err
			return
		}
		if err := migrateLegacyStateFile(dir, statePath); err != nil {
			remoteErr = err
			return
		}
		manager := &RemoteManager{
			statePath:   statePath,
			downloadDir: dir,
			client:      &http.Client{Timeout: 30 * time.Second},
		}
		if err := manager.load(); err != nil {
			remoteErr = err
			return
		}
		remoteInstance = manager
	})
	return remoteInstance, remoteErr
}

func (m *RemoteManager) DownloadDir() string {
	return m.downloadDir
}

func (m *RemoteManager) GetState() RemoteState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *RemoteManager) SetInterval(seconds int) {
	if seconds <= 0 {
		seconds = 300
	}
	m.mu.Lock()
	m.state.IntervalSeconds = seconds
	_ = m.saveLocked()
	m.mu.Unlock()
}

func (m *RemoteManager) AddURLs(urls []string) ([]RemoteSource, error) {
	m.mu.Lock()

	var added []RemoteSource
	seen := make(map[string]bool)
	for _, src := range m.state.Sources {
		seen[src.URL] = true
	}

	for _, raw := range urls {
		normalized, err := normalizeRemoteURL(raw)
		if err != nil {
			continue
		}
		if seen[normalized] {
			continue
		}
		id := hashURL(normalized)
		fileName := buildRemoteFileName(normalized, id)
		filePath := filepath.Join(m.downloadDir, fileName)
		item := RemoteSource{
			ID:       id,
			URL:      normalized,
			FileName: fileName,
			FilePath: filePath,
		}
		m.state.Sources = append(m.state.Sources, item)
		seen[normalized] = true
		added = append(added, item)
	}

	if err := m.saveLocked(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	for i := range added {
		m.download(&added[i], true)
	}

	m.mergeDownloaded(added)
	_ = m.saveLocked()
	return added, nil
}

func (m *RemoteManager) RemoveByID(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := false
	kept := m.state.Sources[:0]
	for _, src := range m.state.Sources {
		if src.ID == id || src.URL == id {
			removed = true
			_ = os.Remove(src.FilePath)
			continue
		}
		kept = append(kept, src)
	}
	m.state.Sources = kept
	_ = m.saveLocked()
	return removed
}

func (m *RemoteManager) CheckUpdates() (int, error) {
	m.mu.Lock()
	sources := make([]RemoteSource, len(m.state.Sources))
	copy(sources, m.state.Sources)
	m.mu.Unlock()

	updated := 0
	for i := range sources {
		if m.download(&sources[i], false) {
			updated++
		}
	}

	m.mergeDownloaded(sources)
	m.mu.Lock()
	err := m.saveLocked()
	m.mu.Unlock()
	return updated, err
}

func (m *RemoteManager) StartUpdateLoop(stop <-chan struct{}) {
	go func() {
		for {
			interval := m.getInterval()
			if interval <= 0 {
				interval = 300
			}
			select {
			case <-time.After(time.Duration(interval) * time.Second):
				if updated, err := m.CheckUpdates(); err != nil {
					logger.Warn("Remote update check failed: %v", err)
				} else if updated > 0 {
					logger.Info("Remote subscriptions updated: %d", updated)
				}
			case <-stop:
				return
			}
		}
	}()
}

func (m *RemoteManager) getInterval() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.IntervalSeconds
}

func (m *RemoteManager) mergeDownloaded(updated []RemoteSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	byID := make(map[string]RemoteSource, len(m.state.Sources))
	for _, src := range m.state.Sources {
		byID[src.ID] = src
	}
	for _, src := range updated {
		if _, ok := byID[src.ID]; ok {
			byID[src.ID] = src
		}
	}
	m.state.Sources = m.state.Sources[:0]
	for _, src := range byID {
		m.state.Sources = append(m.state.Sources, src)
	}
}

func (m *RemoteManager) download(src *RemoteSource, force bool) bool {
	req, err := http.NewRequest("GET", src.URL, nil)
	if err != nil {
		src.Error = err.Error()
		src.LastChecked = time.Now()
		return false
	}
	if !force {
		if src.ETag != "" {
			req.Header.Set("If-None-Match", src.ETag)
		}
		if src.LastModified != "" {
			req.Header.Set("If-Modified-Since", src.LastModified)
		}
	}

	resp, err := m.client.Do(req)
	if err != nil {
		src.Error = err.Error()
		src.LastChecked = time.Now()
		return false
	}
	defer resp.Body.Close()

	src.LastChecked = time.Now()

	if resp.StatusCode == http.StatusNotModified {
		src.Error = ""
		return false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		src.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return false
	}

	tmpPath := src.FilePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		src.Error = err.Error()
		return false
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		src.Error = err.Error()
		return false
	}
	if err := os.Rename(tmpPath, src.FilePath); err != nil {
		src.Error = err.Error()
		return false
	}

	src.ETag = strings.TrimSpace(resp.Header.Get("ETag"))
	src.LastModified = strings.TrimSpace(resp.Header.Get("Last-Modified"))
	src.LastUpdated = time.Now()
	src.Error = ""
	return true
}

func (m *RemoteManager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.IntervalSeconds = 300

	data, err := os.ReadFile(m.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return m.saveLocked()
		}
		return err
	}
	if err := json.Unmarshal(data, &m.state); err != nil {
		return err
	}
	if m.state.IntervalSeconds <= 0 {
		m.state.IntervalSeconds = 300
	}
	return nil
}

func (m *RemoteManager) saveLocked() error {
	payload, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.statePath, payload, 0o644)
}

func normalizeRemoteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme")
	}

	host := strings.ToLower(parsed.Host)
	if host == "github.com" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 5 && parts[2] == "blob" {
			rawHost := "raw.githubusercontent.com"
			rawPath := "/" + strings.Join(append(parts[:2], parts[3:]...), "/")
			parsed.Host = rawHost
			parsed.Path = rawPath
		}
	}

	parsed.Fragment = ""
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func hashURL(input string) string {
	sum := sha1.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

func buildRemoteFileName(u string, id string) string {
	parsed, err := url.Parse(u)
	base := "remote.txt"
	if err == nil {
		name := filepath.Base(parsed.Path)
		if name != "" && name != "/" && name != "." {
			base = name
		}
	}
	if filepath.Ext(base) == "" {
		base += ".txt"
	}
	base = sanitizeFileName(base)
	return fmt.Sprintf("%s_%s", id[:8], base)
}

func sanitizeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "remote.txt"
	}
	return out
}

func remoteStatePath(downloadDir string) string {
	parent := filepath.Dir(filepath.Clean(downloadDir))
	return filepath.Join(parent, ".remote_sources.json")
}

func migrateLegacyStateFile(downloadDir, statePath string) error {
	legacyPath := filepath.Join(downloadDir, ".remote_sources.json")
	if legacyPath == statePath {
		return nil
	}

	if _, err := os.Stat(statePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.Rename(legacyPath, statePath); err == nil {
		return nil
	}

	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		return err
	}
	_ = os.Remove(legacyPath)
	return nil
}
