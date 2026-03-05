# CLI Spec (draft)

## Commands

### `phpvm install <version>`
Install a specific PHP version. Supports partial (`8.2`) and exact (`8.2.17`) versions.

### `phpvm use <version>`
Switch active PHP version to an installed version.

### `phpvm list`
List installed PHP versions.

### `phpvm current`
Show current active PHP version.

### `phpvm available`
Show available PHP versions from upstream source.

### `phpvm remove <version>`
Remove an installed version.

### `phpvm version`
Show phpvm CLI version.
