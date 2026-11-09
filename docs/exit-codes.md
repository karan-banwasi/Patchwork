# Process Exit Codes Reference

Patchwork returns standardized exit codes across all commands, enabling scripts, automation pipelines, and CI/CD workflows to distinguish between operational errors, partial upgrade failures, and user cancellations.

Exit codes are defined in `internal/exitcodes/exitcodes.go`.

---

## 1. Exit Code Table

| Exit Code | Identifier | Meaning | Typical Scenarios |
| :---: | :--- | :--- | :--- |
| `0` | `ExitSuccess` | Success | - `pw check` completed successfully.<br>- All requested package upgrades completed successfully.<br>- `pw upgrade --dry-run` completed.<br>- Package pinned or unpinned successfully.<br>- `pw doctor` or `pw cleanup` completed. |
| `1` | `ExitGeneralError` | Operational / Configuration Error | - Unknown flags or invalid command-line syntax.<br>- Malformed or corrupt `config.json`.<br>- Non-existent package manager passed to `-m`.<br>- All package managers failed during query or network failure.<br>- Non-interactive environment without `--all` or package IDs.<br>- Cryptographic attestation verification failure during `pw self-update`. |
| `2` | `ExitUpgradeFailure` | Upgrade Failure | - `pw upgrade` executed, but **one or more** targeted packages failed to install or upgrade.<br>- Underlying package manager exited with an error code for a package. |
| `3` | `ExitCanceled` | Canceled / Interrupted | - User pressed `Ctrl+C` (SIGINT) to abort the operation.<br>- User canceled interactive selection (submitted empty selection or typed `none`).<br>- Context deadline was exceeded / canceled. |

---

## 2. Using Exit Codes in Automation Scripts

### Bash / Zsh / POSIX Shell
In Unix shells, check the exit status via `$?`:

```bash
pw upgrade --all
STATUS=$?

case $STATUS in
  0)
    echo "All packages upgraded successfully."
    ;;
  1)
    echo "General error occurred: check flags, config, or network." >&2
    exit 1
    ;;
  2)
    echo "One or more packages failed to upgrade." >&2
    exit 2
    ;;
  3)
    echo "Upgrade was canceled by user or signal."
    exit 0
    ;;
esac
```

### PowerShell (Windows / macOS / Linux)
In PowerShell, check `$LASTEXITCODE`:

```powershell
pw upgrade --all
switch ($LASTEXITCODE) {
    0 { Write-Host "All packages upgraded successfully." -ForegroundColor Green }
    1 { Write-Error "General error occurred: check configuration or flags." }
    2 { Write-Warning "Partial upgrade failure: one or more packages failed." }
    3 { Write-Host "Operation was canceled." -ForegroundColor Yellow }
}
```

---

## 3. CI/CD Pipeline Integration

When integrating Patchwork into continuous deployment or configuration management pipelines:
- **Tolerating Partial Upgrades**: If your pipeline should proceed even if an optional package fails to upgrade, test for exit code 2:
  ```bash
  pw upgrade --all || [ $? -eq 2 ]
  ```
- **Strict Verification**: If any failure is unacceptable, require exit code 0:
  ```bash
  pw upgrade --all --dry-run
  if [ $? -ne 0 ]; then
    echo "Pending upgrades detected or check failed!" >&2
    exit 1
  fi
  ```
