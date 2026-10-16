# Deprecation Policy

## Overview

This document describes the deprecation policy for go-project-starter configuration format and features.

## Deprecation Timeline

- **Deprecated features are removed every 2nd minor release**
- Example: feature deprecated in `0.10.x` will be removed in `0.12.0`

## Current Deprecations

No active deprecations.

## Go version policy

Generated projects target a **supported** Go release. Go upstream supports the
two latest minor releases, so the generator default (`tools.golang_version`,
and `MIN_GO_VERSION` in the generated Makefile) tracks them instead of staying
on an older version.

| Change | Release | Note |
|--------|---------|------|
| Generated projects: Go 1.24 → **1.27** | unreleased | `tools.golang_version` default, `MIN_GO_VERSION`, integration image and CI; `goimports` installed from `@latest` again |

**Why a policy and not a one-off bump:** staying on an EOL release only defers
the next breakage. A dependency that raises its own `go` directive (e.g.
`gofrs/uuid/v5` → `go 1.25`, TOOLS-4) breaks `go mod tidy` for everyone on an
older toolchain, and pinning the tool that broke (`goimports@v0.49.0`) moves
the problem rather than fixing it.

**Impact on existing projects:** a project generated earlier keeps the
`tools.golang_version` written in its own `project.yaml` — regeneration does
not change that value. After raising it, the project's build/CI environment
must provide the new toolchain.

**Impact on this repository's own tests:** the generator test configs target
the new version, so the configs with `post_generate: go_mod_tidy` need a Go
1.27 toolchain. With the default `GOTOOLCHAIN=auto` Go downloads it on first
use; with `GOTOOLCHAIN=local` on an older host, `go mod tidy` in the generated
project fails with `go.mod requires go >= 1.27`. The integration-test CI job
installs 1.27.2 explicitly.

## Removed Deprecations

| Feature | Deprecated In | Removed In | Migration |
|---------|---------------|------------|-----------|
| `empty_config_available` | 0.11.0 | 0.13.0 | Use `optional: true` in application transport config |
| Transport string array format | 0.10.0 | 0.12.0 | Use object format: `- name: transport_name` |

## Migration

Run the migrate command to automatically update your config:

```bash
# Check what will be changed (dry-run)
go-project-starter migrate --dry-run

# Apply migration
go-project-starter migrate
```

The migrate command will:
1. Create a backup of your config (`.bak` file)
2. Convert deprecated formats to new formats
3. Report any issues found

## Adding New Deprecations

When adding a new deprecation:

1. Add a new version constant in `internal/pkg/migrate/migrate.go`:
   ```go
   const (
       RemovalVersionTransportStringArray = "0.12.0"
       RemovalVersionNewFeature           = "0.14.0"  // NEW
   )
   ```

2. Add migration logic in `migrate.go`

3. Add warning collection in `config/config.go` `collectDeprecationWarnings()`

4. Update this table with the new deprecation

## Release Checklist

Before each release, check if any deprecations should be removed:

1. Check `internal/pkg/migrate/migrate.go` for `RemovalVersion*` constants
2. If current release version >= removal version:
   - Remove backward compatibility code
   - Remove migration logic for that feature
   - Update this document
