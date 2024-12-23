package workdir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

func isExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func readJson[T any](path string) (*T, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file (%s): %w", path, err)
	}

	var res T
	if err := json.Unmarshal(bb, &res); err != nil {
		return nil, fmt.Errorf("parse file (%s): %w", path, err)
	}

	return &res, nil
}

func writeJson(in any, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "\t")

	if err := enc.Encode(in); err != nil {
		return fmt.Errorf("marshal file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}

func forceReadJson[T any](path string, defVal T) (*T, error) {
	if !isExists(path) {
		if err := writeJson(defVal, path); err != nil {
			return nil, fmt.Errorf("write json to file: %w", err)
		}
	}

	res, err := readJson[T](path)
	if err != nil {
		return nil, fmt.Errorf("read json: %w", err)
	}

	return res, nil
}

func parseSourceURI(uri string) (SourceUri, error) {
	sourceURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse source uri: %w", err)
	}

	switch sourceURL.Scheme {
	default:
		return nil, fmt.Errorf("unsupported source uri scheme (%s)", sourceURL.Scheme)
	case "":
		uri, err := filepath.Abs(uri)
		if err != nil {
			return nil, fmt.Errorf("resolve absolute path: %w", err)
		}

		return SourceUriFile{Path: uri}, nil
	case "http", "https":
		return SourceUriUrl{URL: uri}, nil
	case "git+ssh":
		parts := strings.Split(uri, ":")
		pathToFile := parts[len(parts)-1]

		return SourceUriGit{
			Addr: strings.TrimSuffix(strings.TrimPrefix(uri, "git+ssh://"), ":"+pathToFile),
			Path: pathToFile,
		}, nil
	case "git+https":
		parts := strings.Split(uri, ":")
		pathToFile := parts[len(parts)-1]

		return SourceUriGit{
			Addr: strings.TrimSuffix(strings.TrimPrefix(uri, "git+"), ":"+pathToFile),
			Path: pathToFile,
		}, nil
	}
}

func fetchRemoteSpec(ctx context.Context, source string, tags []string, excluded []string) ([]RemoteSpec, error) {
	{
		if slices.Contains(excluded, source) {
			return []RemoteSpec{}, nil
		}

		excluded = append(excluded, source)
	}

	srcURI, err := parseSourceURI(source)
	if err != nil {
		return nil, fmt.Errorf("parse source uri: %w", err)
	}

	var buf []byte
	switch srcURI := srcURI.(type) {
	default:
		return nil, errors.New("unsupported source uri")
	case SourceUriUrl:
		fmt.Println("Include from url:", srcURI.URL)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURI.URL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch source: %w", err)
		}
		defer resp.Body.Close()

		bb, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}

		buf = bb
	case SourceUriFile:
		fmt.Println("Include from file:", srcURI.Path)

		bb, err := os.ReadFile(srcURI.Path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		buf = bb
	case SourceUriGit:
		fmt.Println("Include from git:", srcURI.Addr, "file:", srcURI.Path)

		targetDir, err := os.MkdirTemp(os.TempDir(), "toolset")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}

		args := []string{
			"clone",
			"--depth", "1",
			srcURI.Addr,
			targetDir,
		}

		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		cmd.Stdin = nil
		cmd.Stdout = io.Discard
		cmdErr := bytes.NewBufferString("")
		cmd.Stderr = cmdErr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("clone git repo (%s): %w", strings.TrimSpace(cmdErr.String()), err)
		}

		targetFile := filepath.Join(targetDir, srcURI.Path)
		bb, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		if err := os.RemoveAll(targetDir); err != nil {
			return nil, fmt.Errorf("remove temp dir: %w", err)
		}

		buf = bb
	}

	var spec Spec
	if err := json.Unmarshal(buf, &spec); err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	var res []RemoteSpec
	for _, inc := range spec.Includes {
		remotes, err := fetchRemoteSpec(ctx, inc.Src, append(slices.Clone(tags), inc.Tags...), excluded)
		if err != nil {
			return nil, fmt.Errorf("fetch one of remotes (%s): %w", inc, err)
		}

		for _, remote := range remotes {
			excluded = append(excluded, remote.Source)
		}

		res = append(res, remotes...)
	}

	return append(res, RemoteSpec{
		Spec:   spec,
		Source: source,
		Tags:   tags,
	}), nil
}
