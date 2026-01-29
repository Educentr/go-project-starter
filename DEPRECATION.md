# Deprecation Policy

## Overview

This document describes the deprecation policy for go-project-starter configuration format and features.

## Deprecation Timeline

- **Deprecated features are removed every 2nd minor release**
- Example: feature deprecated in `0.10.x` will be removed in `0.12.0`

## Current Deprecations

| Feature | Deprecated In | Removal Version | Migration |
|---------|---------------|-----------------|-----------|
| Transport string array format | 0.10.0 | 0.12.0 | Run `go-project-starter migrate` |

### Transport String Array Format

**Old format (deprecated):**
```yaml
applications:
  - name: myapp
    transport:
      - rest_api
      - grpc_api
```

**New format:**
```yaml
applications:
  - name: myapp
    transport:
      - name: rest_api
      - name: grpc_api
        config:
          instantiation: dynamic  # optional per-transport config
```

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
