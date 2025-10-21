package runtimegh

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v75/github"
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

func TestAutoDiscoverAsset(t *testing.T) {
	// Helper function to create a github.ReleaseAsset pointer
	asset := func(name string) *github.ReleaseAsset {
		return &github.ReleaseAsset{Name: github.Ptr(name)}
	}

	t.Run("discovers assets with hyphen separator", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "darwin arm64 with version",
				toolName: "buf",
				version:  "v1.59.0",
				assets: []*github.ReleaseAsset{
					asset("buf-1.59.0-darwin-arm64.tar.gz"),
					asset("buf-1.59.0-linux-amd64.tar.gz"),
					asset("buf-1.59.0-windows-amd64.zip"),
				},
				wantAsset: "buf-1.59.0-darwin-arm64.tar.gz",
			},
			{
				name:     "darwin arm64 without v prefix",
				toolName: "tool",
				version:  "1.2.3",
				assets: []*github.ReleaseAsset{
					asset("tool-1.2.3-darwin-arm64.tar.gz"),
					asset("tool-1.2.3-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.2.3-darwin-arm64.tar.gz",
			},
			{
				name:     "darwin with x86_64 and arm64 options",
				toolName: "mytool",
				version:  "v2.0.0",
				assets: []*github.ReleaseAsset{
					asset("mytool-2.0.0-darwin-arm64.tar.gz"),
					asset("mytool-2.0.0-darwin-x86_64.tar.gz"),
					asset("mytool-2.0.0-linux-x86_64.tar.gz"),
				},
				wantAsset: "mytool-2.0.0-darwin-arm64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("discovers assets with underscore separator", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "trivy style naming",
				toolName: "trivy",
				version:  "v0.67.2",
				assets: []*github.ReleaseAsset{
					asset("trivy_0.67.2_macOS-ARM64.tar.gz"),
					asset("trivy_0.67.2_Linux-64bit.tar.gz"),
					asset("trivy_0.67.2_windows-64bit.zip"),
				},
				wantAsset: "trivy_0.67.2_macOS-ARM64.tar.gz",
			},
			{
				name:     "gitleaks style naming",
				toolName: "gitleaks",
				version:  "v8.28.0",
				assets: []*github.ReleaseAsset{
					asset("gitleaks_8.28.0_darwin_arm64.tar.gz"),
					asset("gitleaks_8.28.0_linux_amd64.tar.gz"),
				},
				wantAsset: "gitleaks_8.28.0_darwin_arm64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("discovers assets without version in filename", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "buf style - no version in filename",
				toolName: "buf",
				version:  "v1.59.0",
				assets: []*github.ReleaseAsset{
					asset("buf-Darwin-arm64.tar.gz"),
					asset("buf-Linux-x86_64.tar.gz"),
					asset("buf-Windows-x86_64.zip"),
				},
				wantAsset: "buf-Darwin-arm64.tar.gz",
			},
			{
				name:     "golangci-lint style",
				toolName: "golangci-lint",
				version:  "v2.5.0",
				assets: []*github.ReleaseAsset{
					asset("golangci-lint-darwin-arm64.tar.gz"),
					asset("golangci-lint-linux-amd64.tar.gz"),
				},
				wantAsset: "golangci-lint-darwin-arm64.tar.gz",
			},
			{
				name:     "tool with capital Darwin",
				toolName: "sometool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("sometool-Darwin-arm64.tar.gz"),
					asset("sometool-Linux-amd64.tar.gz"),
				},
				wantAsset: "sometool-Darwin-arm64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("discovers assets with different OS naming conventions", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "macOS instead of darwin",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-macOS-arm64.tar.gz"),
					asset("tool-1.0.0-Linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-macOS-arm64.tar.gz",
			},
			{
				name:     "OSX naming",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-osx-arm64.tar.gz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-osx-arm64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("discovers assets with different architecture naming", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "aarch64 instead of arm64",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-aarch64.tar.gz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-darwin-aarch64.tar.gz",
			},
			{
				name:     "ARM64 uppercase",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-ARM64.tar.gz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-darwin-ARM64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("discovers assets with different archive formats", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "zip format",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-arm64.zip"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-darwin-arm64.zip",
			},
			{
				name:     "tgz format",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-arm64.tgz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-darwin-arm64.tgz",
			},
			{
				name:     "tar.xz format",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-arm64.tar.xz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-darwin-arm64.tar.xz",
			},
			{
				name:     "tar.bz2 format",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-arm64.tar.bz2"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-darwin-arm64.tar.bz2",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("handles case insensitivity", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "uppercase Darwin and ARM64",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-DARWIN-ARM64.tar.gz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-DARWIN-ARM64.tar.gz",
			},
			{
				name:     "mixed case",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-Darwin-Arm64.tar.gz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
				wantAsset: "tool-1.0.0-Darwin-Arm64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})

	t.Run("returns error when no matching asset found", func(t *testing.T) {
		tests := []struct {
			name     string
			toolName string
			version  string
			assets   []*github.ReleaseAsset
		}{
			{
				name:     "no darwin assets",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-linux-amd64.tar.gz"),
					asset("tool-1.0.0-windows-amd64.zip"),
				},
			},
			{
				name:     "no arm64 assets",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-amd64.tar.gz"),
					asset("tool-1.0.0-linux-amd64.tar.gz"),
				},
			},
			{
				name:     "wrong archive format",
				toolName: "tool",
				version:  "v1.0.0",
				assets: []*github.ReleaseAsset{
					asset("tool-1.0.0-darwin-arm64.exe"),
					asset("tool-1.0.0-darwin-arm64.dmg"),
				},
			},
			{
				name:     "no assets",
				toolName: "tool",
				version:  "v1.0.0",
				assets:   []*github.ReleaseAsset{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.Error(t, err)
				require.ErrorIs(t, err, errAutoDiscover)
				require.Nil(t, result)
			})
		}
	})

	t.Run("real world examples", func(t *testing.T) {
		tests := []struct {
			name      string
			toolName  string
			version   string
			assets    []*github.ReleaseAsset
			wantAsset string
		}{
			{
				name:     "golangci-lint",
				toolName: "golangci-lint",
				version:  "v2.5.0",
				assets: []*github.ReleaseAsset{
					asset("golangci-lint-darwin-amd64.tar.gz"),
					asset("golangci-lint-darwin-arm64.tar.gz"),
					asset("golangci-lint-linux-amd64.tar.gz"),
					asset("golangci-lint-windows-amd64.zip"),
				},
				wantAsset: "golangci-lint-darwin-arm64.tar.gz",
			},
			{
				name:     "buf with Darwin capitalization",
				toolName: "buf",
				version:  "v1.59.0",
				assets: []*github.ReleaseAsset{
					asset("buf-Darwin-arm64.tar.gz"),
					asset("buf-Darwin-x86_64.tar.gz"),
					asset("buf-Linux-aarch64.tar.gz"),
					asset("buf-Linux-x86_64.tar.gz"),
					asset("buf-Windows-arm64.zip"),
					asset("buf-Windows-x86_64.zip"),
				},
				wantAsset: "buf-Darwin-arm64.tar.gz",
			},
			{
				name:     "trivy with macOS and ARM64",
				toolName: "trivy",
				version:  "v0.67.2",
				assets: []*github.ReleaseAsset{
					asset("trivy_0.67.2_macOS-64bit.tar.gz"),
					asset("trivy_0.67.2_macOS-ARM64.tar.gz"),
					asset("trivy_0.67.2_Linux-64bit.tar.gz"),
					asset("trivy_0.67.2_Linux-ARM64.tar.gz"),
				},
				wantAsset: "trivy_0.67.2_macOS-ARM64.tar.gz",
			},
			{
				name:     "gitleaks",
				toolName: "gitleaks",
				version:  "v8.28.0",
				assets: []*github.ReleaseAsset{
					asset("gitleaks_8.28.0_darwin_arm64.tar.gz"),
					asset("gitleaks_8.28.0_darwin_x86_64.tar.gz"),
					asset("gitleaks_8.28.0_linux_x86_64.tar.gz"),
					asset("gitleaks_8.28.0_windows_x86_64.zip"),
				},
				wantAsset: "gitleaks_8.28.0_darwin_arm64.tar.gz",
			},
			{
				name:     "gotestsum",
				toolName: "gotestsum",
				version:  "v1.13.0",
				assets: []*github.ReleaseAsset{
					asset("gotestsum_1.13.0_darwin_amd64.tar.gz"),
					asset("gotestsum_1.13.0_darwin_arm64.tar.gz"),
					asset("gotestsum_1.13.0_linux_amd64.tar.gz"),
				},
				wantAsset: "gotestsum_1.13.0_darwin_arm64.tar.gz",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := autoDiscoverAsset(tt.assets, tt.toolName, tt.version)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantAsset, result.GetName())
			})
		}
	})
}
