## pw upgrade

Upgrade available packages

### Synopsis

Upgrade one or more specific packages by providing their IDs, or upgrade all packages using the --all flag.

```
pw upgrade [package-id...] [flags]
```

### Options

```
      --accept-agreements   Automatically accept winget source and package license agreements (default true)
  -a, --all                 Upgrade all available packages
      --by-manager          Upgrade packages grouped by package manager
      --dry-run             Preview packages that would be upgraded without installing
  -f, --force               Force upgrade of pinned packages
  -h, --help                help for upgrade
      --include-pinned      Upgrade packages even if they are pinned
      --json                Output upgrade results as JSON
  -m, --manager string      Target a specific package manager
      --no-refresh          Skip bucket/repository refresh before checking updates
      --offline             Disable network access and upgrade using only local caches
      --timeout duration    Per-manager timeout when querying updates (0 uses manager defaults)
      --trust-repository    Pass -TrustRepository when updating PowerShell PSResource modules
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

