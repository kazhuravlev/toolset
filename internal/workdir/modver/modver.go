package modver

import "fmt"

const latest = "__latest_version__"

type ModVer struct {
	name string
	ver  string
}

func (m ModVer) Name() string {
	return m.name
}

func (m ModVer) Version() string {
	if m.ver == latest {
		return "latest"
	}

	return m.ver
}

func (m ModVer) IsLatest() bool {
	return m.ver == latest
}

// AsLatest return the same mod but with latest version.
func (m ModVer) AsLatest() ModVer {
	return ModVer{
		name: m.name,
		ver:  latest,
	}
}

// S returns a module@ver like:
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
// github.com/golangci/golangci-lint/cmd/golangci-lint@latest
func (m ModVer) S() string {
	return fmt.Sprintf("%s@%s", m.Name(), m.Version())
}

func NewVer(name, ver string) ModVer {
	if ver == "" {
		panic("version is required")
	}

	return ModVer{
		name: name,
		ver:  ver,
	}
}

func NewLatest(name string) ModVer {
	return NewVer(name, latest)
}
