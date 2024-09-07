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
