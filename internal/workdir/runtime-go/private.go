package runtimego

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/mod/module"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type moduleInfo struct {
	Canonical string // github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
	Module    string // github.com/golangci/golangci-lint/cmd/golangci-lint
	Version   string // v1.55.2
	Program   string // golangci-lint
	IsPrivate bool   // depends on `go env GOPRIVATE`
}

// parse will parse source string and try to extract all details about mentioned golang program.
func parse(str string) (*moduleInfo, error) {
	var canonical, mod, version, program string

	{
		parts := strings.Split(str, at)
		switch len(parts) {
		default:
			return nil, errors.New("invalid format")
		case 1: // have no version. means latest
			version = "latest"
		case 2: // have version. parse it
			version = parts[1]
		}

		mod = parts[0]
		canonical = mod + at + version

		// github.com/user/repo/cmd/program => program
		if strings.Contains(mod, "/cmd/") {
			program = filepath.Base(strings.Split(mod, "/cmd/")[1])
		} else {
			// github.com/user/repo/v3 => repo
			parts := strings.Split(mod, "/")
			lastPart := parts[len(parts)-1]
			if strings.HasPrefix(lastPart, "v") {
				program = parts[len(parts)-2]
			} else {
				program = lastPart
			}
		}

	}

	return &moduleInfo{
		Canonical: canonical,
		Module:    mod,
		Version:   version,
		Program:   program,
		IsPrivate: module.MatchPrefixPatterns(os.Getenv("GOPRIVATE"), mod),
	}, nil
}

type fetchedMod struct {
	Version string `json:"Version"`
}

func fetchLatest(ctx context.Context, link string) (*moduleInfo, error) {
	mod, err := parse(link)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s) string: %w", link, err)
	}

	if mod.IsPrivate {
		return nil, errors.New("private module is not yet supported")
	}

	link = mod.Module

	for {
		// TODO: use a local proxy if configured.
		// Get the latest version
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://proxy.golang.org/%s/@latest", link), nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get go module: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusNotFound {
				parts := strings.Split(link, "/")
				if len(parts) == 1 {
					break
				}

				link = strings.Join(parts[:len(parts)-1], "/")
				continue
			}

			return nil, fmt.Errorf("unable to get module: %s", resp.Status)
		}

		var fMod fetchedMod
		if err := json.NewDecoder(resp.Body).Decode(&fMod); err != nil {
			return nil, fmt.Errorf("unable to decode module: %w", err)
		}

		mod2, err := parse(mod.Module + at + fMod.Version)
		if err != nil {
			return nil, fmt.Errorf("parse fetched module: %w", err)
		}

		return mod2, nil
	}

	return nil, errors.New("unknown module")
}

func isExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}
