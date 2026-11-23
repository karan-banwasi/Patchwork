## pw self-update

Update Patchwork (pw) to the latest release

### Synopsis

Check GitHub Releases for a newer version of Patchwork and update the executable in-place.

```
pw self-update [flags]
```

### Options

```
      --channel string        Release channel to check or update from (stable or prerelease)
      --check                 Check for a newer version without downloading or replacing the binary
  -f, --force                 Force self-update even if installed via Winget or Scoop
  -h, --help                  help for self-update
      --json                  Output self-update status in JSON format
      --offline               Disable network access
      --prerelease            Include prerelease versions (shorthand for --channel prerelease)
      --require-attestation   Require build provenance attestation even in offline/custom environments
      --skip-attestation      Skip build provenance attestation verification
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

