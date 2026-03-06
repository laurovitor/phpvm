# phpvm

`phpvm` is a cross-platform PHP version manager inspired by `nvm`.

Goal: make PHP version switching as easy as:

```bash
phpvm install 8.2
phpvm use 8.2
php -v
```

## Vision

- One CLI UX for **Windows, Linux, and macOS**.
- Per-user installs (no global system breakage).
- Fast switching between PHP versions by project.
- Composer always follows the active PHP version.

## Planned Commands

- `phpvm install <version>` (alias: `i`)
- `phpvm use <version>` (alias: `u`)
- `phpvm list` (alias: `ls`)
- `phpvm current` (alias: `c`)
- `phpvm available` (alias: `a`)
- `phpvm remove <version>` (alias: `rm`)
- `phpvm version` (alias: `v`)
- `phpvm selfupdate` (alias: `su`)

## First Milestone (v0.1)

- Windows + Linux support
- Install from official PHP upstream archives
- `install/use/list/current/version/remove/available`
- PATH/symlink based switching
- Version selectors:
  - `phpvm install 8` -> latest stable `8.x.y`
  - `phpvm install 8.2` -> latest stable `8.2.x`
  - `phpvm install 8.2.30` -> exact version

macOS support lands in the next milestone.

## Why Go?

This project starts in **Go** for:

- easy cross-platform binaries
- simple distribution (single executable)
- strong stdlib for networking/filesystem/process handling

## Status

🚧 Early alpha in progress.

Current implementation supports command flow and version resolution. Archive layout still varies by platform/package, so install/use behavior will be hardened next.

## Windows testing (no paid CI required)

You can test in Windows right now without GitHub Actions:

```bash
# from repo root
./scripts/build_release_local.sh v0.1.0-alpha.local
```

Artifacts will be generated in `dist/` including:

- `phpvm.exe`
- `phpvm-setup.exe`
- `phpvm-windows-amd64.zip`

To create a draft GitHub release manually (using `gh`):

```bash
./scripts/release_windows_local.sh v0.1.0-alpha.1 "First Windows alpha draft"
```

## Windows installer (wizard)

A proper wizard installer script is included at:

- `installer/windows/phpvm.iss`

It supports:

- install/update in `%LocalAppData%\\phpvm`
- optional PATH registration for current user
- uninstall entry in Apps & Features
- PATH cleanup on uninstall

Build instructions are documented in:

- `scripts/build_installer_notes.md`

See [docs/ROADMAP.md](docs/ROADMAP.md) and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## License

MIT — see [LICENSE](LICENSE).
