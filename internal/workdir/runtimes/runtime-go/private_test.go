package runtimego

import (
	"context"
	"github.com/kazhuravlev/toolset/internal/prog"
	"github.com/stretchr/testify/require"
	"os/exec"
	"testing"
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
