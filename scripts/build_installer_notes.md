# Building phpvm-setup.exe (NSIS)

We use **NSIS** so we can build a Windows wizard installer directly from Linux/macOS/Windows.

## Prerequisites

- NSIS installed (`makensis` in PATH)
- `dist/phpvm.exe` exists (from build script)

## Build command

```bash
./scripts/build_windows_setup.sh v0.1.1-alpha.2
```

Expected output:

- `dist/phpvm-setup.exe`

## What installer does

- Installs `phpvm.exe` to `%LocalAppData%\\phpvm\\bin`
- Adds path for current user (idempotent)
- Registers uninstall entry in Apps & Features
- Removes PATH entry on uninstall
