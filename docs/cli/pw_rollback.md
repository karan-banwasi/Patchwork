## pw rollback

Roll back upgraded packages to a previous version

### Synopsis

Roll back one or more packages to their previous version recorded in upgrade history, or to an explicit version with --to.

```
pw rollback [package-id...] [flags]
```

### Options

```
      --dry-run          Preview rollback actions without modifying installed packages
  -h, --help             help for rollback
      --json             Output rollback results as JSON
      --last             Roll back all succeeded packages from the most recent upgrade run
  -m, --manager string   Scope rollback to a specific package manager
      --run string       Roll back all succeeded packages from a specific upgrade run ID
      --to string        Target version to roll back to (single package only)
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

