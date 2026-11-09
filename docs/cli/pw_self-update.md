## pw self-update

Update Patchwork (pw) to the latest release

### Synopsis

Check GitHub Releases for a newer version of Patchwork and update the executable in-place.

```
pw self-update [flags]
```

### Options

```
  -f, --force                 Force self-update even if installed via Winget or Scoop
  -h, --help                  help for self-update
      --offline               Disable network access
      --require-attestation   Require build provenance attestation even in offline/custom environments
      --skip-attestation      Skip build provenance attestation verification
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

