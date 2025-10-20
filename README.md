# Toolset

[![Go Reference](https://pkg.go.dev/badge/github.com/kazhuravlev/toolset.svg)](https://pkg.go.dev/github.com/kazhuravlev/toolset)
[![License](https://img.shields.io/github/license/kazhuravlev/toolset?color=blue)](https://github.com/kazhuravlev/toolset/blob/master/LICENSE)
[![Build Status](https://github.com/kazhuravlev/toolset/actions/workflows/release.yml/badge.svg)](https://github.com/kazhuravlev/toolset/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kazhuravlev/toolset)](https://goreportcard.com/report/github.com/kazhuravlev/toolset)
[![codecov](https://codecov.io/gh/kazhuravlev/toolset/graph/badge.svg?token=CD8TDB2DBU)](https://codecov.io/gh/kazhuravlev/toolset)

`toolset` is a lightweight utility for managing project-specific tools like linters, formatters, and code generators. It
ensures that your development tools remain up-to-date and allows you to specify exact tool versions for consistent,
reproducible builds.

## Key Features

- **Centralized Tool Management**: Easily manage and update tools like linters, formatters, and code generators within
  your project.
- **Reproducible Builds**: Ensure consistent results by locking tool versions in your project configuration.
- **Automatic Updates**: Keep your tools up-to-date with a single command, avoiding manual version checks and upgrades.
- **Multiple Runtimes**: Support for both Go builds and GitHub releases. Use the `gh` runtime to install pre-built
  binaries from GitHub releases (like golangci-lint) for significantly faster installation compared to building from
  source.

## Use Cases

When developing software, you often rely on various tools—such as linters, formatters, and test runners—that are
critical for maintaining code quality. These tools are frequently updated with new features or fixes. To maintain
stability and ensure that everyone working on the project uses the same toolset, you need a way to lock in specific tool
versions.

This is where `toolset` comes in:

- **Version Management**: Specify exact tool versions to avoid breaking changes during updates.
- **Easy Upgrades**: When you're ready, upgrade all tools to their latest versions with a single command.
- **Reproducibility**: Enable consistent behavior across machines, making builds and code checks more reliable.

## Installation

You can install `toolset` using either Go or Homebrew:

### Option 1: Go Install

```shell
go install github.com/kazhuravlev/toolset/cmd/toolset@latest
```

### Option 2: Homebrew

```shell
brew install kazhuravlev/toolset/toolset
```

## Usage

`toolset` allows you to initialize a configuration, add tools, install specific versions, and run or upgrade them. Below
are the basic commands:

### Initialize Toolset

Create a `toolset.json` configuration file in the specified directory:

```shell
# Init an empty project
toolset init .
# ...or from existing one.
toolset init --copy-from git+https://gist.github.com/3f16049ce3f9f478e6b917237b2c0d88.git:/sample-toolset.json
```

### Add Tools

`toolset` supports multiple runtimes for installing tools. Use `go` runtime to build from source, or `gh` runtime to
download pre-built binaries from GitHub releases (faster for tools like golangci-lint).

#### Direct tools

Explicitly add tool to your configuration. They have an priority on top of includes.

```shell
# Add using Go runtime (builds from source)
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.2
# Add the latest version (automatically resolves to the most recent version)
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Add using GitHub releases runtime (downloads pre-built binary - faster!)
toolset add gh golangci/golangci-lint@v1.61.0
```

#### Copy from source

When you bootstrap a new repository and want to have a copy of the `.toolset.json` file from an existing repository.

```shell
# ... from local file
toolset add --copy-from path/to/another/.toolset.json
# ... from remote http
toolset add --copy-from https://gist.githubusercontent.com/kazhuravlev/3f16049ce3f9f478e6b917237b2c0d88/raw/44a2ea7d2817e77e2cd90f29343788c864d36567/sample-toolset.json
# ... from git repo (by ssh)
toolset add --copy-from git+ssh://git@gist.github.com:3f16049ce3f9f478e6b917237b2c0d88.git:/sample-toolset.json
# ... from git repo (by https)
toolset add --copy-from git+https://gist.github.com/3f16049ce3f9f478e6b917237b2c0d88.git:/sample-toolset.json
```

#### Include source

The included source will be explicitly registered. This URL will be added to your `.toolset.json` file.

```shell
# ... from local file
toolset add --include /path/to/.toolset.json
# ... from remote http
toolset add --include https://gist.githubusercontent.com/kazhuravlev/3f16049ce3f9f478e6b917237b2c0d88/raw/44a2ea7d2817e77e2cd90f29343788c864d36567/sample-toolset.json
# ... from git repo (by ssh)
toolset add --include git+ssh://git@gist.github.com:3f16049ce3f9f478e6b917237b2c0d88.git
# ... from git repo (by https)
toolset add --include git+https://gist.github.com/3f16049ce3f9f478e6b917237b2c0d88.git
```

#### Add tags to tools

Add one or more tags to each tool. It will allow you to install only selected tools. Tools that have a tag can be
filtered in `toolset sync` and `toolset upgrade`. See the docs bellow.

```shell
# Add linter to group `linters` and `ci`
toolset add --tags linters,ci go github.com/golangci/golangci-lint/cmd/golangci-lint
# Add tools to group `ci`
toolset add --tags ci go github.com/jstemmer/go-junit-report/v2@latest
toolset add --tags ci go github.com/boumenot/gocover-cobertura
```

#### Add tools from GitHub Releases (gh runtime)

**Important Feature**: `toolset` supports installing tools directly from GitHub releases using the `gh` runtime. This is
particularly useful for tools like `golangci-lint` that provide pre-built binaries, making installation much faster than
building from source.

```shell
# Install golangci-lint from GitHub releases (much faster than building from source)
toolset add gh golangci/golangci-lint@v1.61.0
toolset sync

# Run the tool
toolset run golangci-lint version

# Upgrade to a newer release
toolset upgrade golangci-lint
```

The `gh` runtime downloads pre-compiled binaries from GitHub releases, which is significantly faster than the `go`
runtime that builds from source. This is especially beneficial for large tools like golangci-lint.

**Authentication**: For private repositories or to avoid rate limits, set `GITHUB_TOKEN` or `TOOLSET_GITHUB_TOKEN`
environment variable:

```shell
export GITHUB_TOKEN=ghp_your_token_here
toolset add gh owner/private-repo@v1.0.0
```

#### Use specific golang version

In order to install tool with concrete golang version:

```shell
# Add a new runtime (it will installed at the moment)
toolset runtime add go@1.22.10
# Check that it was added
toolset runtime list
```

And install tool with specific version:

```shell
toolset add go@1.22.10 github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.0
toolset sync
toolset list
```

### Install tools

Ensure all specified tools are installed or updated to the defined versions:

```shell
# Make sure that all tools installed with a correct version.
toolset sync
# ...do it only for some set of tools
toolset sync --tags linters
```

By default, tools are installed into `~/.cache/toolset`.

### Run a tool

Execute any installed tool with its corresponding arguments. For example, to run `golangci-lint`:

```shell
toolset run golangci-lint run --fix ./...
```

### Upgrade tools

To upgrade all tools to their latest available versions, run:

```shell
# Upgrade all tools
toolset upgrade
# Upgrade only specific tools
toolset upgrade --tags ci
```

This command ensures all tools in your toolset.json configuration are updated to the latest version.

### Get an absolute path to installed tool

To get an abs path to installed tool you can just use a `toolset which`:

```shell
# Install some tool. For example - goimports
toolset add go golang.org/x/tools/cmd/goimports
toolset sync
# Get an abs path to installed tool
toolset which goimports
```

This command returns an abs path to goimports like that:

```
/Users/username/.cache/toolset/go1.23/.goimports___v0.21.0/goimports
```

### Remove installed tool

You can use `toolset remove` in very similar order like `toolset add` or `toolset run`. This operation will remove
requested binaries and some meta info.

```shell
# Install some tool. For example - goimports
toolset add go golang.org/x/tools/cmd/goimports
toolset sync
# Remove tool by calling
toolset remove goimports
```

## Examples

Here’s an [example](./example) of a directory with the toolset. To try it out, follow these steps:

[Install](#installation) the toolset and run:

```shell
git clone git@github.com:kazhuravlev/toolset.git
cd toolset/example
# Install all tools
toolset sync
# ... and check installed tools
ls ~/.cache/toolset
# Run installed tool
toolset run gt --repo ../ tag last
# List installed tools
toolset list
# List installed tools that was never used
toolset list --unused
```

## Supported Runtimes

`toolset` supports multiple runtime environments for installing tools:

### Go Runtime (`go`)

Builds tools from Go source code. Supports any Go module that can be built with `go install`.

```shell
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
toolset add go golang.org/x/tools/cmd/goimports@latest
```

**Advantages:**

- Works with any Go module
- Can use specific Go versions for building
- Full source code compilation

**Disadvantages:**

- Slower installation (needs to compile from source)

### GitHub Releases Runtime (`gh`)

Downloads pre-built binaries directly from GitHub releases.

```shell
toolset add gh golangci/golangci-lint@v1.61.0
toolset add gh owner/repository@v2.5.0
```

**Advantages:**

- Much faster installation (downloads pre-compiled binaries)
- No compilation required
- Perfect for large tools like golangci-lint

**Disadvantages:**

- Only works with projects that publish release binaries
- Requires consistent release naming conventions

**When to use which:**

- Use `gh` for tools that provide pre-built binaries (faster)
- Use `go` for tools without releases or when you need specific Go versions

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests to improve toolset.

## Questions and Answers

**Which files should be added into git index?**

- `.toolset.json` should be added into git index. This is like `go.mod` or `package.json`.
- `.toolset.lock.json` should be added into git index. This is like `go.sum` or another lock files.

**Is that possible to change directory that contains a binary files?**

Yes. You can change it in your `.toolset.json`.

**I have a strange behaviour. What I should do to fix that?**

Main command - `toolset sync`. This should fix the all problems. In case when it is not fixed - create an issue.

**Where toolset store tools?**

All tools and stats file stored into `~/.cache/toolset` directory. You can delete this directory any time. Toolset will
download necessary tools at the next run.

**How to change directory where tools stored?**

Just export variable like that `export TOOLSET_CACHE_DIR=/tmp/some-directory`.

**How to change directory where `.toolset.json` and `.toolset.lock.json` stored?**

Just export variable like that `export TOOLSET_SPEC_DIR=.some/directory`. Toolset will try to find spec files into this
dir.

**How do I use GitHub authentication for the gh runtime?**

Set the `GITHUB_TOKEN` or `TOOLSET_GITHUB_TOKEN` environment variable with your GitHub personal access token. This is
useful for:

- Accessing private repositories
- Avoiding GitHub API rate limits
- CI/CD environments

```shell
export GITHUB_TOKEN=ghp_your_token_here
toolset add gh owner/private-repo@v1.0.0
```

**What environment variables does toolset support?**

- `TOOLSET_CACHE_DIR` - Change where tools are stored (default: `~/.cache/toolset`)
- `TOOLSET_SPEC_DIR` - Change where `.toolset.json` and `.toolset.lock.json` are located
- `GITHUB_TOKEN` or `TOOLSET_GITHUB_TOKEN` - GitHub authentication for the `gh` runtime
