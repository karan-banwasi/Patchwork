## pw schedule run

Run a scheduled background check now

### Synopsis

Fetch available updates across active package managers, refresh the local update cache, update schedule state, and optionally send a desktop notification.

```
pw schedule run [flags]
```

### Options

```
  -h, --help               help for run
      --notify             Send a desktop notification when updates are found (default true)
      --offline            Disable network access and check using only local caches
  -q, --quiet              Suppress normal console output
      --timeout duration   Per-manager timeout when querying updates (0 uses manager defaults)
```

### SEE ALSO

* [pw schedule](pw_schedule.md)	 - Manage scheduled background update checks and desktop notifications

