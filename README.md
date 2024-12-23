# Toolset

[![Go Reference](https://pkg.go.dev/badge/github.com/kazhuravlev/toolset.svg)](https://pkg.go.dev/github.com/kazhuravlev/toolset)
[![License](https://img.shields.io/github/license/kazhuravlev/toolset?color=blue)](https://github.com/kazhuravlev/toolset/blob/master/LICENSE)
[![Build Status](https://github.com/kazhuravlev/toolset/actions/workflows/release.yml/badge.svg)](https://github.com/kazhuravlev/toolset/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kazhuravlev/toolset)](https://goreportcard.com/report/github.com/kazhuravlev/toolset)

`toolset` is a lightweight utility for managing project-specific tools like linters, formatters, and code generators. It
ensures that your development tools remain up-to-date and allows you to specify exact tool versions for consistent,
reproducible builds.

## Key Features

- **Centralized Tool Management**: Easily manage and update tools like linters, formatters, and code generators within
  your project.
- **Reproducible Builds**: Ensure consistent results by locking tool versions in your project configuration.
- **Automatic Updates**: Keep your tools up-to-date with a single command, avoiding manual version checks and upgrades.

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
toolset init .
```

### Add Tools

#### Direct tools

Explicitly add tool to your configuration. They have an priority on top of includes.

```shell
# Add the latest version (automatically resolves to the most recent version)
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint
# ... or with @latest
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@latest
# Add a specific version
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.2
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

### Install tools

Ensure all specified tools are installed or updated to the defined versions:

```shell
# Make sure that all tools installed with a correct version.
toolset sync
# ...do it only for some set of tools
toolset sync --tags linters
```

By default, tools are installed into ./bin/tools.

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
/abs/path/to/project/bin/tools/.goimports___v0.21.0/goimports
```

### Remove installed tool

You can use `toolset remove` in very similar order like `toolset add` or `toolset run`. This operation will remove
requested binaries and some meta info.

```shell
# Install some tool. For example - goimports
toolset add go golang.org/x/tools/cmd/goimports
toolset sync
# Remove tool by calling one of next command
toolset remove goimports
toolset remove golang.org/x/tools/cmd/goimports
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
ls ./bin/tools
# Run installed tool
toolset run gt --repo ../ tag last
# List installed tools
toolset list
# List installed tools that was never used
toolset list --unused
```

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests to improve toolset.

## Questions and Answers

**Which files should be added into git index?**

- `.toolset.json` should be added into git index. This is like `go.mod` or `package.json`.
- `.toolset.lock.json` should be added into git index. This is like `go.sum` or another lock files.
- `bin/tools` dir should be excluded from index, because this dir will contain a binaries.

**Is that possible to change directory that contains a binary files?**

Yes. You can change it in your `.toolset.json`.

**I have a strange behaviour. What I should do to fix that?**

Main command - `toolset sync`. This should fix the all problems. In case when it is not fixed - create an issue.

