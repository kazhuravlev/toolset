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

Create a toolset.json configuration file in the specified directory:

```shell
toolset init .
```

### Add Tools

Add tools to your configuration. You can specify the latest version or pin a specific version:

#### Add the latest version (automatically resolves to the most recent version):

```shell
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint
```

#### Add a specific version

```shell
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.2
```

#### Copy from another file

```shell
toolset add --copy-from path/to/another/.toolset.json
```

#### Copy from another url

```shell
toolset add --copy-from https://gist.githubusercontent.com/kazhuravlev/3f16049ce3f9f478e6b917237b2c0d88/raw/44a2ea7d2817e77e2cd90f29343788c864d36567/sample-toolset.json
```

#### Include another url

Included source will be registered explicitly. This url will be added in your `.toolset.json`.

```shell
toolset add --include https://gist.githubusercontent.com/kazhuravlev/3f16049ce3f9f478e6b917237b2c0d88/raw/44a2ea7d2817e77e2cd90f29343788c864d36567/sample-toolset.json
```

### Install or Update Tools

Ensure all specified tools are installed or updated to the defined versions:

```shell
toolset sync
```

By default, tools are installed into ./bin/tools.

### Run a Tool

Execute any installed tool with its corresponding arguments. For example, to run golangci-lint:

```shell
toolset run golangci-lint run --fix ./...
```

### Upgrade Tools

To upgrade all tools to their latest available versions, run:

```shell
toolset upgrade
```

This command ensures all tools in your toolset.json configuration are updated to the latest version.

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
