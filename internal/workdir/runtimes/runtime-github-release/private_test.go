package runtimegh

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/stretchr/testify/require"
)

func TestRuntimeVersion(t *testing.T) {
	runtime := New(fsh.NewMemFS(nil), "/tmp/tools", nil)
	require.Equal(t, "gh", runtime.Version())
}

func TestRuntimeParse(t *testing.T) {
	runtime := New(fsh.NewMemFS(nil), "/tmp/tools", nil)
	ctx := context.Background()

	t.Run("valid module strings", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{
				name:  "golangci-lint",
				input: "golangci/golangci-lint@v1.61.0",
				want:  "golangci/golangci-lint@v1.61.0",
			},
			{
				name:  "tool with different version",
				input: "owner/tool@v2.3.4",
				want:  "owner/tool@v2.3.4",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := runtime.Parse(ctx, tt.input)
				require.NoError(t, err)
				require.Equal(t, tt.want, result)
			})
		}
	})

	t.Run("invalid module strings", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
		}{
			{
				name:  "empty string",
				input: "",
			},
			{
				name:  "missing version",
				input: "owner/repo",
			},
			{
				name:  "invalid semver",
				input: "owner/repo@latest",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := runtime.Parse(ctx, tt.input)
				require.Error(t, err)
				require.Empty(t, result)
			})
		}
	})
}

func TestRuntimeGetModule(t *testing.T) {
	t.Run("constructs correct paths for module", func(t *testing.T) {
		const binToolDir = "/cache/tools"
		memFS := fsh.NewMemFS(nil)
		runtime := New(memFS, binToolDir, nil)
		ctx := context.Background()

		tests := []struct {
			name            string
			module          string
			wantName        string
			wantBinDir      string
			wantBinPath     string
			wantModuleS     string
			wantIsInstalled bool
		}{
			{
				name:            "golangci-lint",
				module:          "golangci/golangci-lint@v1.61.0",
				wantName:        "golangci-lint",
				wantBinDir:      filepath.Join(binToolDir, "gh/golangci/golangci-lint@v1.61.0"),
				wantBinPath:     filepath.Join(binToolDir, "gh/golangci/golangci-lint@v1.61.0/golangci-lint"),
				wantModuleS:     "golangci/golangci-lint@v1.61.0",
				wantIsInstalled: false,
			},
			{
				name:            "different tool",
				module:          "owner/mytool@v2.0.0",
				wantName:        "mytool",
				wantBinDir:      filepath.Join(binToolDir, "gh/owner/mytool@v2.0.0"),
				wantBinPath:     filepath.Join(binToolDir, "gh/owner/mytool@v2.0.0/mytool"),
				wantModuleS:     "owner/mytool@v2.0.0",
				wantIsInstalled: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				moduleInfo, err := runtime.GetModule(ctx, tt.module)
				require.NoError(t, err)
				require.NotNil(t, moduleInfo)
				require.Equal(t, tt.wantName, moduleInfo.Name)
				require.Equal(t, tt.wantBinDir, moduleInfo.BinDir)
				require.Equal(t, tt.wantBinPath, moduleInfo.BinPath)
				require.Equal(t, tt.wantModuleS, moduleInfo.Mod.S())
				require.Equal(t, tt.wantIsInstalled, moduleInfo.IsInstalled)
				require.False(t, moduleInfo.IsPrivate)
			})
		}
	})

	t.Run("detects installed binary", func(t *testing.T) {
		const binToolDir = "/cache/tools"
		// Create a filesystem with the binary already present
		binaryPath := filepath.Join(binToolDir, "gh/golangci/golangci-lint@v1.61.0/golangci-lint")
		memFS := fsh.NewMemFS(map[string]string{
			binaryPath: "fake binary content",
		})
		runtime := New(memFS, binToolDir, nil)
		ctx := context.Background()

		moduleInfo, err := runtime.GetModule(ctx, "golangci/golangci-lint@v1.61.0")
		require.NoError(t, err)
		require.NotNil(t, moduleInfo)
		require.True(t, moduleInfo.IsInstalled, "should detect existing binary")
	})

	t.Run("returns error for invalid module", func(t *testing.T) {
		runtime := New(fsh.NewMemFS(nil), "/cache/tools", nil)
		ctx := context.Background()

		moduleInfo, err := runtime.GetModule(ctx, "invalid-module")
		require.Error(t, err)
		require.Nil(t, moduleInfo)
	})
}

func TestParse(t *testing.T) {
	t.Run("valid GitHub releases", func(t *testing.T) {
		tests := []struct {
			name        string
			input       string
			wantOwner   string
			wantRepo    string
			wantVersion string
		}{
			{
				name:        "golangci-lint",
				input:       "golangci/golangci-lint@v1.61.0",
				wantOwner:   "golangci",
				wantRepo:    "golangci-lint",
				wantVersion: "v1.61.0",
			},
			{
				name:        "another tool",
				input:       "owner/repo@v2.5.0",
				wantOwner:   "owner",
				wantRepo:    "repo",
				wantVersion: "v2.5.0",
			},
			{
				name:        "tool with patch version",
				input:       "someowner/sometool@v1.2.3",
				wantOwner:   "someowner",
				wantRepo:    "sometool",
				wantVersion: "v1.2.3",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := parse(tt.input)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantRepo, result.Program)
				require.Equal(t, tt.wantOwner+"/"+tt.wantRepo, result.Mod.Name())
				require.Equal(t, tt.wantVersion, result.Mod.Version())
			})
		}
	})

	t.Run("invalid inputs", func(t *testing.T) {
		tests := []struct {
			name    string
			input   string
			wantErr string
		}{
			{
				name:    "empty string",
				input:   "",
				wantErr: "program name not provided",
			},
			{
				name:    "missing version",
				input:   "owner/repo",
				wantErr: "invalid github repository: should be owner/proj@v1.2.3",
			},
			{
				name:    "missing owner",
				input:   "repo@v1.0.0",
				wantErr: "invalid github path: should be owner/proj",
			},
			{
				name:    "too many slashes",
				input:   "owner/sub/repo@v1.0.0",
				wantErr: "invalid github path: should be owner/proj",
			},
			{
				name:    "invalid semver",
				input:   "owner/repo@1.0.0",
				wantErr: "non-semver versions is not supported",
			},
			{
				name:    "invalid semver format",
				input:   "owner/repo@latest",
				wantErr: "non-semver versions is not supported",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := parse(tt.input)
				require.Error(t, err)
				require.Nil(t, result)
				require.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})
}
