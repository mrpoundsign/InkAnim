package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultReleaseURL points to the GitHub Releases latest endpoint for InkAnim.
const DefaultReleaseURL = "https://api.github.com/repos/mrpoundsign/InkAnim/releases/latest"

// UpdateResult contains the version check results and metadata.
type UpdateResult struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	IsOutdated     bool   `json:"is_outdated"`
	ReleaseURL     string `json:"release_url"`
	ReleaseNotes   string `json:"release_notes"`
}

// githubRelease represents the subset of GitHub Release API response we inspect.
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Message string `json:"message"`
}

// CheckForUpdate queries the GitHub Releases API to check if a newer version exists.
func CheckForUpdate(currentVersion string, client *http.Client) (*UpdateResult, error) {
	return CheckForUpdateWithURL(currentVersion, DefaultReleaseURL, client)
}

// CheckForUpdateWithURL checks for updates against a specified API endpoint (useful for testing).
func CheckForUpdateWithURL(currentVersion, apiURL string, client *http.Client) (*UpdateResult, error) {
	if client == nil {
		client = &http.Client{
			Timeout: 5 * time.Second,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update request: %w", err)
	}

	req.Header.Set("User-Agent", "InkAnim-UpdateChecker")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("update check timed out after 5 seconds")
		}
		return nil, fmt.Errorf("network error during update check: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("GitHub API rate limit exceeded; please try again later")
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no releases found for repository")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release information: %w", err)
	}

	latestVersion := release.TagName
	if latestVersion == "" {
		return nil, errors.New("release response contained no version tag")
	}

	// Compare current version with latest version
	cmp, err := CompareSemver(currentVersion, latestVersion)
	if err != nil {
		// If current is "dev" or invalid, treat as outdated if latest is valid, but don't fail hard
		if strings.EqualFold(strings.TrimSpace(currentVersion), "dev") {
			return &UpdateResult{
				CurrentVersion: currentVersion,
				LatestVersion:  latestVersion,
				IsOutdated:     false,
				ReleaseURL:     release.HTMLURL,
				ReleaseNotes:   release.Body,
			}, nil
		}
		return nil, fmt.Errorf("version comparison error: %w", err)
	}

	return &UpdateResult{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		IsOutdated:     cmp < 0,
		ReleaseURL:     release.HTMLURL,
		ReleaseNotes:   release.Body,
	}, nil
}

// CheckForUpdateAsync runs CheckForUpdate on a background goroutine and invokes callback with results.
func CheckForUpdateAsync(currentVersion string, client *http.Client, callback func(*UpdateResult, error)) {
	go func() {
		res, err := CheckForUpdate(currentVersion, client)
		if callback != nil {
			callback(res, err)
		}
	}()
}

// SemVer represents parsed semantic version components.
type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
}

// ParseSemver parses a semantic version string (with optional 'v' prefix).
func ParseSemver(v string) (SemVer, error) {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")

	if s == "" {
		return SemVer{}, errors.New("empty version string")
	}

	var prerelease string
	if idx := strings.IndexAny(s, "-+"); idx != -1 {
		prerelease = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) > 3 || len(parts) == 0 {
		return SemVer{}, fmt.Errorf("invalid semver format %q", v)
	}

	nums := make([]int, 3)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return SemVer{}, fmt.Errorf("invalid semver number %q in %q: %w", part, v, err)
		}
		if n < 0 {
			return SemVer{}, fmt.Errorf("negative semver number %d in %q", n, v)
		}
		nums[i] = n
	}

	return SemVer{
		Major:      nums[0],
		Minor:      nums[1],
		Patch:      nums[2],
		Prerelease: prerelease,
	}, nil
}

// CompareSemver compares two semantic version strings.
// Returns:
//   -1 if v1 < v2  (v1 is older than v2)
//    0 if v1 == v2
//    1 if v1 > v2  (v1 is newer than v2)
func CompareSemver(v1, v2 string) (int, error) {
	s1, err := ParseSemver(v1)
	if err != nil {
		return 0, fmt.Errorf("failed to parse current version %q: %w", v1, err)
	}
	s2, err := ParseSemver(v2)
	if err != nil {
		return 0, fmt.Errorf("failed to parse target version %q: %w", v2, err)
	}

	if s1.Major != s2.Major {
		if s1.Major < s2.Major {
			return -1, nil
		}
		return 1, nil
	}

	if s1.Minor != s2.Minor {
		if s1.Minor < s2.Minor {
			return -1, nil
		}
		return 1, nil
	}

	if s1.Patch != s2.Patch {
		if s1.Patch < s2.Patch {
			return -1, nil
		}
		return 1, nil
	}

	// Major.Minor.Patch are equal; compare prerelease metadata
	if s1.Prerelease == "" && s2.Prerelease == "" {
		return 0, nil
	}
	// Normal version has higher precedence than prerelease (1.0.0 > 1.0.0-beta)
	if s1.Prerelease != "" && s2.Prerelease == "" {
		return -1, nil
	}
	if s1.Prerelease == "" && s2.Prerelease != "" {
		return 1, nil
	}

	// Both have prerelease strings: compare lexicographically
	if s1.Prerelease < s2.Prerelease {
		return -1, nil
	}
	if s1.Prerelease > s2.Prerelease {
		return 1, nil
	}

	return 0, nil
}
