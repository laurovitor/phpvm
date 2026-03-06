# Building phpvm-setup.exe (Windows)

We use **Inno Setup** for a proper Windows wizard installer with uninstall support.

## Prerequisites

- Install Inno Setup 6 on Windows
- Ensure `dist/phpvm.exe` exists (from build)

## Build command (PowerShell)

```powershell
iscc installer\windows\phpvm.iss
```

Expected output:

- `dist/phpvm-setup.exe`

## What installer does

- Installs `phpvm.exe` to `%LocalAppData%\\phpvm\\bin`
- Optional add-to-PATH task (current user)
- Registers uninstall entry in Apps & Features
- Removes PATH entry on uninstall
