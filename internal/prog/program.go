package prog

import "fmt"

const latest = "__latest_version__"

type Version struct {
	name string
	ver  string
}

func NewVer(name, ver string) Version {
	if ver == "" {
		panic("version is required")
	}

	return Version{
		name: name,
		ver:  ver,
	}
}

func NewLatest(name string) Version {
	return NewVer(name, latest)
}

func (m Version) Name() string {
	return m.name
}

func (m Version) Version() string {
	if m.IsLatest() {
		return "latest"
	}

	return m.ver
}

func (m Version) IsLatest() bool {
	return m.ver == latest
}

// AsLatest return the same mod but with latest version.
func (m Version) AsLatest() Version {
	return Version{
		name: m.name,
		ver:  latest,
	}
}

// S returns a module@ver like:
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
// github.com/golangci/golangci-lint/cmd/golangci-lint@latest
func (m Version) S() string {
	return fmt.Sprintf("%s@%s", m.Name(), m.Version())
}
