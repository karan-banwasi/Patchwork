## pw check

Check for available updates

### Synopsis

Iterate through supported package managers and check for available updates without installing them.

```
pw check [flags]
```

### Options

```
      --by-manager         Group updates by package manager
      --changelog          Display changelog/release preview URLs for available updates
      --exclude strings    Exclude packages matching glob pattern (comma-separated or repeatable)
  -h, --help               help for check
      --include-pinned     Include pinned packages in the check output
      --json               Output available updates as JSON
      --level string       Maximum semver bump level to include (patch, minor, major)
  -m, --manager string     Target a specific package manager
      --no-refresh         Skip bucket/repository refresh before checking updates
      --offline            Disable network access and check using only local caches
  -q, --quiet              Output only tab-separated package IDs and versions
      --timeout duration   Per-manager timeout when querying updates (0 uses manager defaults)
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

