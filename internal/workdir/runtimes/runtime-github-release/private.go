package runtimegh

import (
	"errors"
	"strings"

	"github.com/kazhuravlev/toolset/internal/prog"
	"golang.org/x/mod/semver"
)

type moduleInfo struct {
	Mod prog.Version

	Program string // golangci-lint
}

func parse(str string) (*moduleInfo, error) {
	if str == "" {
		return nil, errors.New("program name not provided")
	}

	repo, ver, ok := strings.Cut(str, at)
	if !ok {
		return nil, errors.New("invalid github repository: should be owner/proj@v1.2.3")
	}

	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, errors.New("invalid github path: should be owner/proj")
	}

	if !semver.IsValid(ver) {
		return nil, errors.New("non-semver versions is not supported")
	}

	modVer := prog.NewVer(repo, ver)

	return &moduleInfo{
		Mod:     modVer,
		Program: parts[1],
	}, nil
}
