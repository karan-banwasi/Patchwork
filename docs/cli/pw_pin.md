## pw pin

Pin one or more packages to ignore them during check and upgrade

### Synopsis

Add one or more package IDs (or manager-scoped entries like winget:Git.Git) to the Patchwork ignore list so they are skipped during 'pw check' and 'pw upgrade'.

```
pw pin <package-id> [package-id...] [flags]
```

### Options

```
      --expires string   Expiration date or duration for pin (e.g. '2026-12-31', '30d')
  -h, --help             help for pin
      --reason string    Reason for pinning this package
      --version string   Version constraint for pin (e.g. '<23', '~22.0.0')
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

