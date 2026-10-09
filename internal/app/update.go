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

// DefaultReleaseURL points to the GitHub Releases endpoint for InkAnim.
const DefaultReleaseURL = "https://api.github.com/repos/mrpoundsign/InkAnim/releases"

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
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Body       string `json:"body"`
	Message    string `json:"message"`
	Draft      bool   `json:"draft"`
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

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse release information: %w", err)
	}

	if len(releases) == 0 {
		return nil, errors.New("no releases found in repository")
	}

	bestRelease, err := findBestUpdateCandidate(currentVersion, releases)
	if err != nil {
		// If current is "dev" or invalid, treat as up-to-date or use first release if we just want to show something
		if strings.EqualFold(strings.TrimSpace(currentVersion), "dev") {
			return &UpdateResult{
				CurrentVersion: currentVersion,
				LatestVersion:  releases[0].TagName,
				IsOutdated:     false,
				ReleaseURL:     releases[0].HTMLURL,
				ReleaseNotes:   releases[0].Body,
			}, nil
		}
		return nil, fmt.Errorf("version comparison error: %w", err)
	}

	if bestRelease == nil {
		return &UpdateResult{
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
			IsOutdated:     false,
		}, nil
	}

	return &UpdateResult{
		CurrentVersion: currentVersion,
		LatestVersion:  bestRelease.TagName,
		IsOutdated:     true,
		ReleaseURL:     bestRelease.HTMLURL,
		ReleaseNotes:   bestRelease.Body,
	}, nil
}

func findBestUpdateCandidate(currentVersion string, releases []githubRelease) (*githubRelease, error) {
	curSem, err := ParseSemver(currentVersion)
	if err != nil {
		return nil, err
	}
	currentIsStable := curSem.Prerelease == ""

	var newerStables []githubRelease
	var newerPrereleases []githubRelease

	for _, rel := range releases {
		if rel.TagName == "" || rel.Draft {
			continue
		}
		cmp, err := CompareSemver(rel.TagName, currentVersion)
		if err != nil {
			continue
		}
		if cmp > 0 {
			relSem, err := ParseSemver(rel.TagName)
			if err == nil {
				if relSem.Prerelease == "" {
					newerStables = append(newerStables, rel)
				} else {
					newerPrereleases = append(newerPrereleases, rel)
				}
			}
		}
	}

	findHighest := func(list []githubRelease) *githubRelease {
		if len(list) == 0 {
			return nil
		}
		best := &list[0]
		for i := 1; i < len(list); i++ {
			cmp, _ := CompareSemver(list[i].TagName, best.TagName)
			if cmp > 0 {
				best = &list[i]
			}
		}
		return best
	}

	if len(newerStables) > 0 {
		return findHighest(newerStables), nil
	}

	if currentIsStable {
		return nil, nil // Stable does not upgrade to prerelease
	}

	if len(newerPrereleases) > 0 {
		return findHighest(newerPrereleases), nil
	}

	return nil, nil
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
