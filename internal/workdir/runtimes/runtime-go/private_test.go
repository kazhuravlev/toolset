package runtimego

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"

	"github.com/kazhuravlev/toolset/internal/prog"
	"github.com/stretchr/testify/require"
)

func Test_parse(t *testing.T) {
	goBin, err := exec.LookPath("go")
	require.NoError(t, err, "install go")

	f := func(name, in string, exp moduleInfo) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mod, err := parse(ctx, goBin, in)
			require.NoError(t, err)
			require.NotEmpty(t, mod)
			require.Equal(t, exp, *mod)
		})
	}

	f("wo_version", "golang.org/x/tools/cmd/goimports", moduleInfo{
		Mod:       prog.NewLatest("golang.org/x/tools/cmd/goimports"),
		Program:   "goimports",
		IsPrivate: false,
	})
	f("github_latest", "github.com/kisielk/errcheck@latest", moduleInfo{
		Mod:       prog.NewLatest("github.com/kisielk/errcheck"),
		Program:   "errcheck",
		IsPrivate: false,
	})
	f("with_ver", "github.com/bufbuild/buf/cmd/buf@v1.47.2", moduleInfo{
		Mod:       prog.NewVer("github.com/bufbuild/buf/cmd/buf", "v1.47.2"),
		Program:   "buf",
		IsPrivate: false,
	})
	f("v2_version", "github.com/goreleaser/goreleaser/v2", moduleInfo{
		Mod:       prog.NewLatest("github.com/goreleaser/goreleaser/v2"),
		Program:   "goreleaser",
		IsPrivate: false,
	})
}

func Test_fetchModule(t *testing.T) {
	fs := fsh.NewRealFS()
	goBin, err := exec.LookPath("go")
	require.NoError(t, err, "install go")

	f := func(name, link string, exp moduleInfo) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			mod, err := fetchModule(ctx, fs, goBin, link)
			require.NoError(t, err)
			require.NotEmpty(t, mod)
			require.Equal(t, exp, *mod)
		})
	}

	f("with_ver", "github.com/bufbuild/buf/cmd/buf@v1.47.2", moduleInfo{
		Mod:       prog.NewVer("github.com/bufbuild/buf/cmd/buf", "v1.47.2"),
		Program:   "buf",
		IsPrivate: false,
	})
	f("v2_version", "github.com/goreleaser/goreleaser/v2@v2.5.1", moduleInfo{
		Mod:       prog.NewVer("github.com/goreleaser/goreleaser/v2", "v2.5.1"),
		Program:   "goreleaser",
		IsPrivate: false,
	})
}

func Test_getGoVersion(t *testing.T) {
	goBin, err := exec.LookPath("go")
	require.NoError(t, err, "install go")

	ctx := context.Background()
	goVersion, err := getGoVersion(ctx, goBin)
	require.NoError(t, err)
	require.NotEmpty(t, goVersion)
	require.True(t, strings.HasPrefix(goVersion, "1.")) // NOTE(zhuravlev): should looks like 1.23.4
}

func Test_setupGolangciLintCache(t *testing.T) {
	fs := fsh.NewRealFS()
	runtime := New(fs, "/tmp/bin", "/usr/local/go/bin/go", "1.23.4")

	t.Run("user_defined_cache", func(t *testing.T) {
		t.Setenv("GOLANGCI_LINT_CACHE", "/home/user/.cache/toolset/golangci-lint") // Set user-defined cache directory

		env := runtime.setupGolangCILintCache()
		require.NotEmpty(t, env)

		var found bool // Find GOLANGCI_LINT_CACHE in the environment variables
		for _, envVar := range env {
			if strings.HasPrefix(envVar, "GOLANGCI_LINT_CACHE=") {
				require.Equal(t, "GOLANGCI_LINT_CACHE=/home/user/.cache/toolset/golangci-lint/go-1.23.4", envVar)
				found = true
				break
			}
		}

		require.True(t, found, "GOLANGCI_LINT_CACHE should be set")
	})

	t.Run("no_user_cache", func(t *testing.T) {
		t.Setenv("GOLANGCI_LINT_CACHE", "") // Clear user-defined cache directory

		env := runtime.setupGolangCILintCache()
		require.NotEmpty(t, env)

		var found bool // Find GOLANGCI_LINT_CACHE in the environment variables
		for _, envVar := range env {
			if strings.HasPrefix(envVar, "GOLANGCI_LINT_CACHE=") {
				require.Contains(t, envVar, ".cache/golangci-lint/go-1.23.4")
				found = true
				break
			}
		}

		require.True(t, found, "GOLANGCI_LINT_CACHE should be set")
	})
}
