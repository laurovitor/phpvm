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

- `phpvm install <version>`
- `phpvm use <version>`
- `phpvm list`
- `phpvm current`
- `phpvm available`
- `phpvm remove <version>`
- `phpvm version`

## First Milestone (v0.1)

- Windows + Linux support
- Install from official PHP binary sources
- `install/use/list/current/version/remove/available`
- PATH/symlink based switching

macOS support lands in the next milestone.

## Why Go?

This project starts in **Go** for:

- easy cross-platform binaries
- simple distribution (single executable)
- strong stdlib for networking/filesystem/process handling

## Status

🚧 Initial scaffold in progress.

See [docs/ROADMAP.md](docs/ROADMAP.md) and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## License

MIT — see [LICENSE](LICENSE).
