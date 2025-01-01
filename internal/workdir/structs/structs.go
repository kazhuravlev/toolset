package structs

import (
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"slices"
	"strconv"
	"strings"
)

var ErrToolNotInstalled = errors.New("tool not installed")

type RunError struct {
	ExitCode int
}

func (e RunError) Error() string { return strconv.Itoa(e.ExitCode) }

type Tool struct {
	// Name of runtime
	Runtime string `json:"runtime"`
	// Path to module with version
	Module string `json:"module"`
	// Alias create a link in tools. Works like exposing some tools
	Alias optional.Val[string] `json:"alias"`
	Tags  []string             `json:"tags"`
}

func (t Tool) ID() string {
	return fmt.Sprintf("%s:%s", t.Runtime, t.Module)
}

func (t Tool) IsSame(tool Tool) bool {
	if t.Runtime != tool.Runtime {
		return false
	}

	// FIXME(zhuravlev): make it runtime-agnostic

	m1 := strings.Split(t.Module, "@")[0]
	m2 := strings.Split(tool.Module, "@")[0]

	return m1 == m2
}

type Tools []Tool

func (tools *Tools) Add(tool Tool) bool {
	for _, t := range *tools {
		if t.IsSame(tool) {
			return false
		}
	}

	*tools = append(*tools, tool)

	return true
}

func (tools *Tools) AddOrUpdateTool(tool Tool) {
	for i, t := range *tools {
		if t.IsSame(tool) {
			(*tools)[i] = tool
			return
		}
	}

	*tools = append(*tools, tool)
}

func (tools *Tools) Filter(tags []string) Tools {
	if len(tags) == 0 {
		return *tools
	}

	res := make(Tools, 0)

	for _, t := range *tools {
		isTarget := slices.ContainsFunc(t.Tags, func(tag string) bool {
			return slices.Contains(tags, tag)
		})
		if !isTarget {
			continue
		}

		res = append(res, t)
	}

	return res
}

func (tools *Tools) Remove(tool Tool) bool {
	for i, t := range *tools {
		if t.IsSame(tool) {
			*tools = slices.Delete(*tools, i, i+1)
			return true
		}
	}

	return false
}

type ModuleInfo struct {
	Name        string // golangci-lint
	Version     string // v1.61.0
	BinDir      string // /home/user/bin/tools/.golangci-lint__v1.1.1
	BinPath     string // /home/user/bin/tools/.golangci-lint__v1.1.1/golangci-lint
	IsInstalled bool
	IsPrivate   bool
}
