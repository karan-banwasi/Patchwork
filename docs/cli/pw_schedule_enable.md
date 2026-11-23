## pw schedule enable

Enable periodic background update checks

### Synopsis

Register a user-level scheduled task to run 'pw schedule run' periodically and persist the schedule configuration.

```
pw schedule enable [flags]
```

### Options

```
  -h, --help              help for enable
      --interval string   Check interval (hourly, daily, weekly, or Go duration >= 15m) (default "6h")
      --notify            Send desktop notifications when updates are available (default true)
```

### SEE ALSO

* [pw schedule](pw_schedule.md)	 - Manage scheduled background update checks and desktop notifications

