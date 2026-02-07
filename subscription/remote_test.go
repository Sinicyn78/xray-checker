package subscription

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoteStatePathUsesParentDirectory(t *testing.T) {
	root := t.TempDir()
	downloadDir := filepath.Join(root, "subscriptions")

	got := remoteStatePath(downloadDir)
	want := filepath.Join(root, ".remote_sources.json")

	if got != want {
		t.Fatalf("unexpected state path: got %q, want %q", got, want)
	}
}

func TestMigrateLegacyStateFile(t *testing.T) {
	root := t.TempDir()
	downloadDir := filepath.Join(root, "subscriptions")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	legacyPath := filepath.Join(downloadDir, ".remote_sources.json")
	statePath := remoteStatePath(downloadDir)
	payload := []byte(`{"intervalSeconds":300,"sources":[]}`)
	if err := os.WriteFile(legacyPath, payload, 0o644); err != nil {
		t.Fatalf("write legacy state failed: %v", err)
	}

	if err := migrateLegacyStateFile(downloadDir, statePath); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy state file must be removed after migration, stat err: %v", err)
	}
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read migrated state failed: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("migrated payload mismatch: got %q, want %q", string(got), string(payload))
	}
}

func TestAddURLsDoesNotDeadlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vmess://example"))
	}))
	defer server.Close()

	root := t.TempDir()
	downloadDir := filepath.Join(root, "subscriptions")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	manager := &RemoteManager{
		statePath:   filepath.Join(root, ".remote_sources.json"),
		downloadDir: downloadDir,
		client:      server.Client(),
		state: RemoteState{
			IntervalSeconds: 300,
			Sources:         []RemoteSource{},
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := manager.AddURLs([]string{server.URL + "/remote.txt"})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AddURLs failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AddURLs timed out, possible deadlock")
	}
}
