package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1      string
		v2      string
		want    int
		wantErr bool
	}{
		{"v0.1.2", "v0.1.3", -1, false},
		{"v0.1.3", "v0.1.2", 1, false},
		{"v0.1.2", "v0.1.2", 0, false},
		{"0.1.2", "v0.1.2", 0, false},
		{"v0.1.2", "v0.2.0", -1, false},
		{"v0.2.0", "v0.1.2", 1, false},
		{"v1.0.0", "v0.9.9", 1, false},
		{"v0.9.9", "v1.0.0", -1, false},
		{"v0.1.2-beta", "v0.1.2", -1, false},
		{"v0.1.2", "v0.1.2-beta", 1, false},
		{"v0.1.2-alpha", "v0.1.2-beta", -1, false},
		{"v0.1.2-beta", "v0.1.2-alpha", 1, false},
		{"v0.1.2-rc1", "v0.1.2-rc1", 0, false},
		{"v1.2", "v1.2.0", 0, false},
		{"v1", "v1.0.0", 0, false},
		{"", "v0.1.2", 0, true},
		{"invalid", "v0.1.2", 0, true},
		{"v0.1.2", "not-a-version", 0, true},
		{"1.2.3.4", "1.2.3", 0, true},
		{"1.-2.3", "1.2.3", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			got, err := CompareSemver(tt.v1, tt.v2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CompareSemver(%q, %q) error = %v, wantErr %v", tt.v1, tt.v2, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestCheckForUpdateWithURL(t *testing.T) {
	t.Run("Newer release available", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") != "InkAnim-UpdateChecker" {
				t.Errorf("expected User-Agent InkAnim-UpdateChecker, got %s", r.Header.Get("User-Agent"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v0.1.3",
				HTMLURL: "https://github.com/mrpoundsign/InkAnim/releases/tag/v0.1.3",
				Body:    "Fixed crop boundary issues and added update checking.",
			})
		}))
		defer ts.Close()

		res, err := CheckForUpdateWithURL("v0.1.2", ts.URL, ts.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsOutdated {
			t.Errorf("expected IsOutdated to be true for v0.1.2 vs v0.1.3")
		}
		if res.LatestVersion != "v0.1.3" {
			t.Errorf("expected LatestVersion v0.1.3, got %s", res.LatestVersion)
		}
		if res.ReleaseURL != "https://github.com/mrpoundsign/InkAnim/releases/tag/v0.1.3" {
			t.Errorf("expected ReleaseURL, got %s", res.ReleaseURL)
		}
		if !strings.Contains(res.ReleaseNotes, "crop boundary") {
			t.Errorf("expected ReleaseNotes to contain 'crop boundary', got %s", res.ReleaseNotes)
		}
	})

	t.Run("Already on latest release", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v0.1.2",
				HTMLURL: "https://github.com/mrpoundsign/InkAnim/releases/tag/v0.1.2",
				Body:    "Release notes for v0.1.2",
			})
		}))
		defer ts.Close()

		res, err := CheckForUpdateWithURL("v0.1.2", ts.URL, ts.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsOutdated {
			t.Errorf("expected IsOutdated to be false when up to date")
		}
	})

	t.Run("Current version is dev", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v0.1.2",
				HTMLURL: "https://github.com/mrpoundsign/InkAnim/releases/tag/v0.1.2",
			})
		}))
		defer ts.Close()

		res, err := CheckForUpdateWithURL("dev", ts.URL, ts.Client())
		if err != nil {
			t.Fatalf("unexpected error for dev version: %v", err)
		}
		if res.IsOutdated {
			t.Errorf("expected dev build to not report outdated by default")
		}
	})

	t.Run("GitHub rate limit 403", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "API rate limit exceeded",
			})
		}))
		defer ts.Close()

		_, err := CheckForUpdateWithURL("v0.1.2", ts.URL, ts.Client())
		if err == nil {
			t.Fatal("expected rate limit error, got nil")
		}
		if !strings.Contains(err.Error(), "rate limit") {
			t.Errorf("expected rate limit in error message, got: %v", err)
		}
	})

	t.Run("Malformed JSON", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not json"))
		}))
		defer ts.Close()

		_, err := CheckForUpdateWithURL("v0.1.2", ts.URL, ts.Client())
		if err == nil {
			t.Fatal("expected json parse error, got nil")
		}
	})

	t.Run("Server timeout", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		client := &http.Client{Timeout: 20 * time.Millisecond}
		_, err := CheckForUpdateWithURL("v0.1.2", ts.URL, client)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})
}

func TestCheckForUpdateAsync(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v0.1.3",
			HTMLURL: "https://github.com/mrpoundsign/InkAnim/releases/tag/v0.1.3",
		})
	}))
	defer ts.Close()

	// Replace DefaultReleaseURL during test would be one way, but CheckForUpdate calls CheckForUpdateWithURL.
	// We can test async mechanics with a custom goroutine or using a local test helper.
	var wg sync.WaitGroup
	wg.Add(1)

	var receivedResult *UpdateResult
	var receivedErr error

	go func() {
		defer wg.Done()
		receivedResult, receivedErr = CheckForUpdateWithURL("v0.1.2", ts.URL, ts.Client())
	}()

	wg.Wait()

	if receivedErr != nil {
		t.Fatalf("unexpected async error: %v", receivedErr)
	}
	if !receivedResult.IsOutdated {
		t.Errorf("expected IsOutdated true")
	}
}
