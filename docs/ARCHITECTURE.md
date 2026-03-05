# Architecture (draft)

## Core concepts

- **Version Store**: per-user directory with installed PHP versions.
- **Current Pointer**: symlink/junction named `current` to active version.
- **Resolver**: normalizes user inputs (`8.2` -> latest `8.2.x`).
- **Provider**: OS-specific adapter for fetching official PHP binaries.

## Suggested layout

### Linux/macOS

- `~/.phpvm/bin/phpvm`
- `~/.phpvm/versions/<version>/`
- `~/.phpvm/current -> ~/.phpvm/versions/<version>`

### Windows

- `%USERPROFILE%\\.phpvm\\phpvm.exe`
- `%USERPROFILE%\\.phpvm\\versions\\<version>\\`
- `%USERPROFILE%\\.phpvm\\current` (junction/symlink)

## Command flow examples

### install

1. Resolve requested version (`8.2` -> `8.2.17`).
2. Fetch archive from provider.
3. Verify archive integrity (where possible).
4. Extract to versions directory.
5. Mark installed metadata.

### use

1. Validate installed target.
2. Update `current` pointer.
3. Print shell reload instructions if needed.

## Language choice

Go was chosen for cross-platform distribution simplicity and predictable CLI behavior.
