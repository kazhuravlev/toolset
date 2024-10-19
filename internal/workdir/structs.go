package workdir

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"slices"
	"strings"
	"time"
)

type SourceUri interface {
	isSourceUri()
}

type SourceUriFile struct {
	Path string
}

func (SourceUriFile) isSourceUri() {}

type SourceUriUrl struct {
	URL string
}

func (SourceUriUrl) isSourceUri() {}

type SourceUriGit struct {
	Addr string
	Path string
}

func (SourceUriGit) isSourceUri() {}

type GoModule struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
	Origin  struct {
		VCS  string `json:"VCS"`
		URL  string `json:"URL"`
		Hash string `json:"Hash"`
		Ref  string `json:"Ref"`
	} `json:"Origin"`
}

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
	if t.Runtime != runtimeGo {
		panic("not implemented")
	}

	if t.Runtime != tool.Runtime {
		return false
	}

	m1 := strings.Split(t.Module, "@")[0]
	m2 := strings.Split(tool.Module, "@")[0]

	return m1 == m2
}

type Include struct {
	Src  string   `json:"src"`
	Tags []string `json:"tags"`
}

func (i Include) IsSame(include Include) bool {
	return i.Src == include.Src
}

func (i *Include) UnmarshalJSON(bb []byte) error {
	var incStruct struct {
		Src  string   `json:"src"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(bb, &incStruct); err != nil {
		// NOTE: Migration: probably this is an old version of include. This version is just a string.
		var inc string
		if errStr := json.Unmarshal(bb, &inc); errStr != nil {
			return fmt.Errorf("unmarshal Include: %w", errors.Join(err, errStr))
		}

		i.Src = inc
		i.Tags = []string{}
		return nil
	}

	*i = incStruct

	return nil
}

type Lock struct {
	Tools   Tools        `json:"tools"`
	Remotes []RemoteSpec `json:"remotes"`
}

type RemoteSpec struct {
	Source string   `json:"Source"` // TODO(zhuravlev): make it lowercase, add migration
	Spec   Spec     `json:"Spec"`
	Tags   []string `json:"Tags"`
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

type Spec struct {
	Dir      string    `json:"dir"`
	Tools    Tools     `json:"tools"`
	Includes []Include `json:"includes"`
}

func (s *Spec) AddInclude(include Include) bool {
	for _, inc := range s.Includes {
		if inc.IsSame(include) {
			return false
		}
	}

	s.Includes = append(s.Includes, include)
	return true
}
