package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

var (
	fullRe    = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	partialRe = regexp.MustCompile(`^(\d+)\.(\d+)$`)
)

// ResolvePartialVersion resolves a partial version (e.g., "1.25") to the latest patch version.
// Returns the full version string without "go" prefix (e.g., "1.25.4").
func ResolvePartialVersion(ctx context.Context, partialVer string) (string, error) {
	major, minor, _, isPartial, err := parseVersion(partialVer)
	if err != nil {
		return "", err
	}

	if !isPartial {
		// Already a full version, return as-is
		return strings.TrimPrefix(partialVer, "go"), nil
	}

	versions, err := listAvailableVersions(ctx)
	if err != nil {
		return "", fmt.Errorf("list versions: %w", err)
	}

	// Filter versions that match major.minor and are stable
	var candidates []string
	prefix := fmt.Sprintf("go%d.%d.", major, minor)
	for _, v := range versions {
		if !v.Stable {
			continue
		}
		if strings.HasPrefix(v.Version, prefix) {
			candidates = append(candidates, v.Version)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no stable versions found for go%d.%d", major, minor)
	}

	// Sort versions using semver to find the latest
	sort.Slice(candidates, func(i, j int) bool {
		return semver.Compare(candidates[i], candidates[j]) > 0
	})

	latest := candidates[0]
	// Remove "go" prefix
	return strings.TrimPrefix(latest, "go"), nil
}

// goVersionRec represents a Go release version.
type goVersionRec struct {
	Version string `json:"version"` // e.g., "go1.25.4"
	Stable  bool   `json:"stable"`
}

// listAvailableVersions fetches all available Go versions from golang.org.
func listAvailableVersions(ctx context.Context) ([]goVersionRec, error) {
	const url = "https://go.dev/dl/?mode=json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var versions []goVersionRec
	if err := json.Unmarshal(body, &versions); err != nil {
		return nil, fmt.Errorf("parse versions: %w", err)
	}

	return versions, nil
}

// parseVersion parses a version string and returns whether it's a partial version (major.minor)
// or full version (major.minor.patch).
// Returns: major, minor, patch, isPartial
func parseVersion(ver string) (major, minor, patch int, isPartial bool, err error) {
	// Remove "go" prefix if present
	ver = strings.TrimPrefix(ver, "go")

	// Try to match major.minor.patch
	if matches := fullRe.FindStringSubmatch(ver); matches != nil {
		var majorPart, minorPart, patchPart int
		fmt.Sscanf(matches[1], "%d", &majorPart)
		fmt.Sscanf(matches[2], "%d", &minorPart)
		fmt.Sscanf(matches[3], "%d", &patchPart)

		return majorPart, minorPart, patchPart, false, nil
	}

	// Try to match major.minor
	if matches := partialRe.FindStringSubmatch(ver); matches != nil {
		var majorPart, minorPart int
		fmt.Sscanf(matches[1], "%d", &majorPart)
		fmt.Sscanf(matches[2], "%d", &minorPart)

		return majorPart, minorPart, 0, true, nil
	}

	return 0, 0, 0, false, fmt.Errorf("invalid version format: %s", ver)
}
