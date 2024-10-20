package structs

import (
	"github.com/kazhuravlev/optional"
	"slices"
	"strings"
)

type Tool struct {
	// Name of runtime
	Runtime string `json:"runtime"`
	// Path to module with version
	Module string `json:"module"`
	// Alias create a link in tools. Works like exposing some tools
	Alias optional.Val[string] `json:"alias"`
	Tags  []string             `json:"tags"`
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

type ModuleInfo struct {
	Name        string // golangci-lint
	Version     string // v1.61.0
	BinDir      string // /home/user/bin/tools/.golangci-lint__v1.1.1
	BinPath     string // /home/user/bin/tools/.golangci-lint__v1.1.1/golangci-lint
	IsInstalled bool
}
