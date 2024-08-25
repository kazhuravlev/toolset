# toolset

[![Go Reference](https://pkg.go.dev/badge/github.com/kazhuravlev/toolset.svg)](https://pkg.go.dev/github.com/kazhuravlev/toolset)
[![License](https://img.shields.io/github/license/kazhuravlev/toolset?color=blue)](https://github.com/kazhuravlev/toolset/blob/master/LICENSE)
[![Build Status](https://github.com/kazhuravlev/toolset/actions/workflows/release.yml/badge.svg)](https://github.com/kazhuravlev/toolset/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kazhuravlev/toolset)](https://goreportcard.com/report/github.com/kazhuravlev/toolset)

A small program that keep up-to-date project-specific tools like linters, formatters, test-runners.

## Use Cases

When developing a project, you need a set of tools like linters, formatters, and code generators. These tools
periodically receive updates and new versions. If you want to maintain a `reproducible build` process, you must specify
the exact version of each tool you're using. This is where a `toolset` comes in. It allows you to specify the tools and
their versions and manage them and their updates in a straightforward manner.

## Installation

```shell
go install github.com/kazhuravlev/toolset/cmd/toolset@latest
```

## Usage

```shell
# Init toolset
## This will create a toolset.json config in specified directory.
toolset init .

# Add tool
## With latest version (will be replaced to current latest)
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint
## With latest version (will be replaced to current latest)
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@latest
## ... or - with specific 
toolset add go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.2

# Ensure all tools is installed
## This will install all tools into ./bin/tools or upgrade to toe specified versions.
toolset sync

# Run tool
## This will run installed tool and send to this tool all arguments, stdIn/Out/Err.
toolset run golangci-lint run --fix ./...
```

