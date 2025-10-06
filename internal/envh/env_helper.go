package envh

import (
	"os"
)

// GetAllOverride will set some envs and return all envs together. Go-specific.
// This is also will cleanup all envs from host installation except the goprivate and some other important vars.
//
// Example variables that not excluded
// GOEXPERIMENT='rangefunc'
// GONOPROXY='github.com/example/provate'
// GONOSUMDB='github.com/example/provate'
// GOPRIVATE='github.com/example/provate'
// GOPROXY='https://proxy.golang.org,direct'
// GOSUMDB='sum.golang.org'
// GOTELEMETRY='local'
func GetAllOverride(envs [][2]string) []string {
	excluded := []string{
		"AR",
		"CC",
		"CGO_CFLAGS",
		"CGO_CPPFLAGS",
		"CGO_CXXFLAGS",
		"CGO_ENABLED",
		"CGO_FFLAGS",
		"CGO_LDFLAGS",
		"CXX",
		"GCCGO",
		"GO111MODULE",
		"GOARCH",
		"GOARM64",
		"GOAUTH",
		"GOBIN",
		"GOCACHE",
		"GOCACHEPROG",
		"GODEBUG",
		"GOENV",
		"GOEXE",
		//"GOEXPERIMENT",
		"GOFIPS140",
		"GOFLAGS",
		"GOGCCFLAGS",
		"GOHOSTARCH",
		"GOHOSTOS",
		"GOINSECURE",
		"GOMOD",
		"GOMODCACHE",
		//"GONOPROXY",
		//"GONOSUMDB",
		"GOOS",
		"GOPATH",
		//"GOPRIVATE",
		//"GOPROXY",
		"GOROOT",
		//"GOSUMDB",
		//"GOTELEMETRY",
		"GOTELEMETRYDIR",
		"GOTMPDIR",
		"GOTOOLCHAIN",
		"GOTOOLDIR",
		"GOVCS",
		"GOVERSION",
		"GOWORK",
		"PKG_CONFIG",
	}

	for _, env := range excluded {
		os.Unsetenv(env)
	}

	for _, pair := range envs {
		key := pair[0]
		val := pair[1]
		os.Setenv(key, val)
	}

	return os.Environ()
}
