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
		return nil, fmt.Errorf("auto-discover: %w", err)
	}

	return targetAsset, nil
}

func autoDiscoverAsset(assets []*github.ReleaseAsset, toolName, version string) (*github.ReleaseAsset, error) {
	osName := map[string]string{
		"darwin":  "darwin",
		"linux":   "linux",
		"windows": "windows",
		"freebsd": "freebsd",
	}[runtime.GOOS]

	archName := map[string]string{
		"amd64": "amd64|x86_64",
		"arm64": "arm64|aarch64",
		"386":   "386|x86|i386",
		"arm":   "armv6|armv7|arm",
	}[runtime.GOARCH]

	if osName == "" || archName == "" {
		return nil, fmt.Errorf("unsupported local platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	pattern := fmt.Sprintf(`(?i)^%s[-_]?(v)?%s[-_]?%s[-_]?(%s)(\.tar\.gz|\.zip|\.tgz|\.tar\.xz|\.tar\.bz2)$`,
		regexp.QuoteMeta(toolName),
		strings.TrimPrefix(version, "v"),
		osName,
		archName,
	)

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regexp compile: %w", err)
	}

	for _, asset := range assets {
		if re.MatchString(asset.GetName()) {
			return asset, nil
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
