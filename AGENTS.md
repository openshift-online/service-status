# AGENTS.md

This file provides guidance to AI coding assistants when working with this repository.

## Project Overview

Service Status — a project to harvest information about managed services and render it. Collects and displays the operational status of Red Hat managed OpenShift services.

## Build & Test Commands

```bash
make build           # Build the binary
make test            # Run tests
make images          # Build container images
make clean           # Remove build artifacts
```

## Architecture

- **cmd/**: Application entry point
- **pkg/**: Core logic for service status collection and rendering
- **vendor/**: Vendored Go dependencies

## Key Conventions

- Module path: `github.com/openshift-online/service-status`
- Uses vendored dependencies (`vendor/` directory)
