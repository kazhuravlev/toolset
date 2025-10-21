package runtimegh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/google/go-github/v75/github"
)

var errAutoDiscover = errors.New("auto-discover")

func (r *Runtime) getAsset(ctx context.Context, owner string, repo string, tag string) (*github.ReleaseAsset, error) {
	release, _, err := r.github.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("get release by tag: %w", err)
	}

	targetAsset, err := autoDiscoverAsset(release.Assets, repo, tag)
	if err != nil {
		if errors.Is(err, errAutoDiscover) {
			var assetNames []string
			for _, asset := range release.Assets {
				assetNames = append(assetNames, asset.GetName())
			}
			return nil, fmt.Errorf("could not auto-discover compatible asset for %s/%s (platform: %s/%s). Available assets: %v",
				owner, repo, runtime.GOOS, runtime.GOARCH, assetNames)
		}
		return nil, fmt.Errorf("auto-discover asset: %w", err)
	}

	return targetAsset, nil
}

func autoDiscoverAsset(assets []*github.ReleaseAsset, toolName, version string) (*github.ReleaseAsset, error) {
	// Map of OS names used in different release naming conventions
	osNames := map[string][]string{
		"darwin":  {"darwin", "macOS", "macos", "osx", "Darwin"},
		"linux":   {"linux", "Linux"},
		"windows": {"windows", "Windows"},
		"freebsd": {"freebsd", "FreeBSD"},
	}[runtime.GOOS]

	// Map of architecture names used in different release naming conventions
	archNames := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64", "64bit"},
		"arm64": {"arm64", "aarch64", "ARM64"},
		"386":   {"386", "x86", "i386", "32bit"},
		"arm":   {"armv6", "armv7", "arm", "ARM"},
	}[runtime.GOARCH]

	if len(osNames) == 0 || len(archNames) == 0 {
		return nil, fmt.Errorf("unsupported local platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	// Build regex patterns to try (in order of preference)
	patterns := []string{
		// Pattern 1: toolname-v1.0.0-darwin-arm64.tar.gz
		fmt.Sprintf(`(?i)^%s[-_](v)?%s[-_](%s)[-_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.xz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.TrimPrefix(version, "v"),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 2: toolname-darwin-arm64.tar.gz (no version)
		fmt.Sprintf(`(?i)^%s[-_](%s)[-_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.xz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 3: toolname_v1.0.0_darwin_arm64.tar.gz (underscores)
		fmt.Sprintf(`(?i)^%s[_](v)?%s[_](%s)[_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.TrimPrefix(version, "v"),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
		// Pattern 4: toolname_darwin_arm64.tar.gz (underscores, no version)
		fmt.Sprintf(`(?i)^%s[_](%s)[_](%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.bz2)$`,
			regexp.QuoteMeta(toolName),
			strings.Join(osNames, "|"),
			strings.Join(archNames, "|")),
	}

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("regexp compile: %w", err)
		}

		for _, asset := range assets {
			if re.MatchString(asset.GetName()) {
				return asset, nil
			}
		}
	}

	return nil, errAutoDiscover
}

func (r *Runtime) downloadAsset(ctx context.Context, owner string, repo string, assetID int64, targetFile string) error {
	body, _, err := r.github.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, http.DefaultClient)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer body.Close() //nolint:errcheck

	target, err := r.fs.OpenFile(targetFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer target.Close() //nolint:errcheck

	if _, err := io.Copy(target, body); err != nil {
		return fmt.Errorf("copy body to file: %w", err)
	}

	return nil
}
