# Patchwork
A CLI tool to better manage package updates. Built natively in Go for Windows.

## Features
- **Check for Updates**: Fetch a perfectly aligned, tabular list of all available package updates.
- **Interactive Upgrades**: Select exactly which packages you want to upgrade using an intuitive checkbox interface directly in your terminal.
- **Bulk Upgrades**: Use the `--all` flag to automatically apply all pending updates at once.
- **Targeted Upgrades**: Upgrade a single app instantly by passing its Package ID.
- **Winget Integration**: Leverages Windows Package Manager (`winget`) under the hood, parsing its output robustly to provide a seamless CLI experience.

## Installation
Ensure you have Go installed on your system.

Clone the repository and run:
```powershell
go mod tidy
go build -o pw.exe main.go
```

### Global Installation (Optional)
If you want to run `pw` from anywhere without using the `.\` prefix, you can build the executable directly into your Go binaries folder (which is typically already on your system's PATH):

```powershell
go build -o $env:USERPROFILE\go\bin\pw.exe main.go
```
Now you can simply use `pw check` and `pw upgrade` globally.

## Development

### Code Generation (`go-winres`)
This project uses [`go-winres`](https://github.com/tc-hib/go-winres) to embed the Windows application manifest and icon into the binary. The pre-built `.syso` files (`rsrc_windows_amd64.syso`, `rsrc_windows_386.syso`) are committed to the repository, so a normal `go build` works out-of-the-box.

If you modify `winres/winres.json` or replace `winres/icon.png`, you must regenerate them:

```powershell
# Regenerate the .syso resource files
go generate ./...
```

### Indirect dependency notes (`golang.org/x/image`, `golang.org/x/text`)

`go.mod` pins `golang.org/x/image v0.39.0` and `golang.org/x/text v0.36.0`
as indirect dependencies. Their version selection is non-obvious, so here is
the full rationale:

**`golang.org/x/text`** — genuine production indirect dependency:
```
AlecAivazis/survey/v2  →  x/text           (character-width, Unicode case-folding)
                           survey needs ≥ v0.4.0
```

**`golang.org/x/image`** — tools-only indirect dependency:
```
go-winres v0.3.3  →  tc-hib/winres v0.2.1  →  x/image  (PNG/icon processing)
                      winres needs ≥ v0.12.0
```

**Why are both pinned so high (v0.39.0 / v0.36.0)?**

`x/image v0.39.0` is the latest release MVS selects. Its own `go.mod` declares
`go 1.25.0` and requires `x/text v0.36.0`. Because `go mod tidy` must respect
the highest `go` directive across the transitive graph, our module's `go`
directive is also pegged to `1.25.0`, and `x/text` is bumped from `v0.4.0`
(survey's floor) to `v0.36.0` (x/image's floor).

Neither bump introduces a breaking change for Patchwork. `x/image` is never
linked into the production binary — it is only reachable via the
`//go:build tools` tag in `tools.go`. You can verify this:

```powershell
go mod why golang.org/x/image
# → (main module does not need package golang.org/x/image)

go mod why -m golang.org/x/text
# → github.com/karan-banwasi/patchwork/cmd → survey/v2 → x/text/cases
```

## Usage

> **Note:** If you did not perform the **Global Installation** above, you will need to prefix the commands with `.\` (e.g., `.\pw check`) if you are running them from the project directory in PowerShell.

**Check for available updates:**
```powershell
pw check
```

**Open the interactive upgrade menu:**
```powershell
pw upgrade
```

**Upgrade a specific package by ID:**
```powershell
pw upgrade Microsoft.VisualStudioCode
```

**Upgrade all available packages:**
```powershell
pw upgrade --all
```
