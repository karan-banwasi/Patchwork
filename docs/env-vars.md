# Environment Variables Reference

This document lists all environment variables recognized, inspected, or honored by Patchwork across supported platforms.

---

## 1. Core Patchwork Configuration & Cache

| Variable | Type | Default Value | Description |
| :--- | :--- | :--- | :--- |
| `PATCHWORK_CONFIG` | Path | `%APPDATA%\Patchwork\config.json` (Windows)<br>`~/Library/Application Support/Patchwork/config.json` (macOS)<br>`~/.config/Patchwork/config.json` (Linux) | Overrides the location of the primary configuration file (used for persistent package pins and options). |
| `PATCHWORK_CACHE` | Path | `%LOCALAPPDATA%\Patchwork\updates-cache.json` (Windows)<br>`~/Library/Caches/Patchwork/updates-cache.json` (macOS)<br>`~/.cache/Patchwork/updates-cache.json` (Linux) | Overrides the location of the short-lived update check completion cache. |
| `PATCHWORK_OFFLINE` | Boolean (`1`, `true`) | Unset | When set to `1` or `true`, forces offline mode equivalent to passing the `--offline` CLI flag. |

---

## 2. GitHub API Authentication (Self-Update)

| Variable | Type | Default Value | Description |
| :--- | :--- | :--- | :--- |
| `GITHUB_TOKEN` | Secret String | Unset | Classic or fine-grained GitHub Personal Access Token. Used by `pw self-update` to query releases against `api.github.com`. Increases API rate limit from 60 requests/hr (unauthenticated) to 5,000 requests/hr. |
| `GH_TOKEN` | Secret String | Unset | Alternative fallback for `GITHUB_TOKEN` (standard GitHub CLI environment variable). |

---

## 3. Network Proxy Configuration

Patchwork's HTTP client (`internal/httputil`) and Go's standard networking library honor standard proxy environment variables:

| Variable | Type | Description |
| :--- | :--- | :--- |
| `HTTP_PROXY` / `http_proxy` | URL | Proxy server URL for unencrypted HTTP traffic (e.g. `http://proxy.corp.internal:8080`). |
| `HTTPS_PROXY` / `https_proxy` | URL | Proxy server URL for secure HTTPS traffic (e.g. `http://proxy.corp.internal:8443`). |
| `NO_PROXY` / `no_proxy` | Comma-separated list | Comma-separated hostnames or IP ranges exempt from proxy routing (e.g. `localhost,127.0.0.1,.internal.net`). |

---

## 4. Operating System & Manager Discovery

Patchwork and its underlying package manager drivers inspect standard operating system environment variables during discovery and execution:

| Variable | Target Platform | Description |
| :--- | :--- | :--- |
| `PATH` | All | Scanned by `exec.LookPath` to determine availability and resolve absolute binary locations for all package manager CLIs (`brew`, `winget`, `scoop`, `choco`, `npm`, `dotnet`, `uv`, `cargo`, `pwsh`). Also verified by `pw doctor`. |
| `SCOOP` | Windows | Root directory of user Scoop installation (default: `%USERPROFILE%\scoop`). |
| `SCOOP_GLOBAL` | Windows | Root directory of global Scoop installation (default: `C:\ProgramData\scoop`). |
| `USERPROFILE` | Windows | User home directory used to resolve Scoop fallback paths and user go bin directory. |
| `ProgramData` | Windows | System program data directory used to locate Chocolatey executable fallback (`C:\ProgramData\chocolatey\bin\choco.exe`). |
| `HOME` | macOS & Linux | User home directory used to locate Homebrew / Linuxbrew user installations (`~/.linuxbrew`, `~/homebrew`). |
| `LOCALAPPDATA` | Windows | Local application data directory used for default cache location and winget isolation. |
| `APPDATA` | Windows | Roaming application data directory used for default configuration storage (`%APPDATA%\Patchwork\config.json`). |
| `SystemRoot` | Windows | Windows system root (typically `C:\Windows`) preserved when executing elevated or isolated subprocesses. |

---

## 5. Security Best Practices

- **Token Safety**: When using `GITHUB_TOKEN` or `GH_TOKEN`, ensure the token only has read access to public repositories. Never grant write access or administrator permissions to tokens used solely for self-updating.
- **Path Tampering**: Never include untrusted directories in your system `PATH` to avoid binary hijacking.
