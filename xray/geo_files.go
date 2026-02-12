package xray

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"xray-checker/logger"
)

const (
	geoSiteFile = "geo/geosite.dat"
	geoIPFile   = "geo/geoip.dat"

	geoDownloadTimeout = 90 * time.Second
	geoDownloadRetries = 3
)

var (
	geoSiteURLs = []string{
		"https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat",
	}
	geoIPURLs = []string{
		"https://github.com/v2fly/geoip/releases/latest/download/geoip.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat",
	}
)

type GeoFileManager struct {
	baseDir    string
	httpClient *http.Client
}

func NewGeoFileManager(baseDir string) *GeoFileManager {
	if baseDir == "" {
		if wd, err := os.Getwd(); err == nil {
			baseDir = wd
		} else {
			baseDir = "."
		}
	}

	return &GeoFileManager{
		baseDir: baseDir,
		httpClient: &http.Client{
			Timeout: geoDownloadTimeout,
		},
	}
}

func (gfm *GeoFileManager) EnsureGeoFiles() error {
	if err := gfm.ensureFile(geoSiteFile, geoSiteURLs); err != nil {
		return fmt.Errorf("failed to ensure geosite.dat: %v", err)
	}

	if err := gfm.ensureFile(geoIPFile, geoIPURLs); err != nil {
		return fmt.Errorf("failed to ensure geoip.dat: %v", err)
	}

	return nil
}

func (gfm *GeoFileManager) ensureFile(filename string, urls []string) error {
	filePath := filepath.Join(gfm.baseDir, filename)

	if _, err := os.Stat(filePath); err == nil {
		return nil
	}

	logger.Info("Downloading %s...", filename)

	fileDir := filepath.Dir(filePath)
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	if err := gfm.downloadWithFallback(urls, filePath); err != nil {
		return fmt.Errorf("failed to download %s: %v", filename, err)
	}

	logger.Info("Downloaded %s", filename)
	return nil
}

func (gfm *GeoFileManager) downloadWithFallback(urls []string, filePath string) error {
	if len(urls) == 0 {
		return errors.New("no download URLs configured")
	}

	var lastErr error
	for _, u := range urls {
		for attempt := 1; attempt <= geoDownloadRetries; attempt++ {
			if attempt > 1 {
				// Simple linear backoff to avoid instant repeated failures.
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			if err := gfm.downloadFile(u, filePath); err != nil {
				lastErr = err
				logger.Warn("Geo download failed (%s, attempt %d/%d): %v", u, attempt, geoDownloadRetries, err)
				continue
			}
			return nil
		}
	}

	if lastErr == nil {
		lastErr = errors.New("all download attempts failed")
	}
	return lastErr
}

func (gfm *GeoFileManager) downloadFile(url, filePath string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("request build failed: %v", err)
	}
	req.Header.Set("User-Agent", "xray-checker/geo-downloader")

	resp, err := gfm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	tmpPath := filePath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}

	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write file: %v", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close file: %v", closeErr)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize file: %v", err)
	}

	return nil
}
