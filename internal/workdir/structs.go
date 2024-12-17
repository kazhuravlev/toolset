package workdir

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
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
	Tools   structs.Tools `json:"tools"`
	Remotes []RemoteSpec  `json:"remotes"`
}

func (l *Lock) FromSpec(spec *Spec) {
	l.Tools = make(structs.Tools, 0, len(spec.Tools))
	for _, tool := range spec.Tools {
		l.Tools.Add(tool)
	}

	// TODO(zhuravlev): should we refresh remotes from spec?

	for _, remote := range l.Remotes {
		for _, tool := range remote.Spec.Tools {
			tool.Tags = append(tool.Tags, remote.Tags...)
			l.Tools.Add(tool)
		}
	}
}

type Stats struct {
	Version string               `json:"version"`
	Tools   map[string]time.Time `json:"tools"`
}

type RemoteSpec struct {
	Source string   `json:"source"`
	Spec   Spec     `json:"spec"`
	Tags   []string `json:"tags"`
}

func (r *RemoteSpec) UnmarshalJSON(bb []byte) error {
	// NOTE(zhuravlev): Migration: from Tags to tags
	var spec struct {
		Source string   `json:"source"`
		Spec   Spec     `json:"spec"`
		Tags   []string `json:"tags"`
	}
	if err := json.Unmarshal(bb, &spec); err != nil {
		var specOld struct {
			Source string   `json:"Source"`
			Spec   Spec     `json:"Spec"`
			Tags   []string `json:"Tags"`
		}
		if errOld := json.Unmarshal(bb, &specOld); errOld != nil {
			return fmt.Errorf("unmarshal RemoteSpec: %w", errors.Join(err, errOld))
		}

		*r = RemoteSpec(specOld)
		return nil
	}

	*r = spec

	return nil
}

type Spec struct {
	// This dir is store all toolset-related files.
	// This directory should be managed by toolset only.
	Dir      string        `json:"dir"`
	Tools    structs.Tools `json:"tools"`
	Includes []Include     `json:"includes"`
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
