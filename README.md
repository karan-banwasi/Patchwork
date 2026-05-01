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
go build -o patchwork.exe main.go
```

## Usage

**Check for available updates:**
```powershell
patchwork check
```

**Open the interactive upgrade menu:**
```powershell
patchwork upgrade
```

**Upgrade a specific package by ID:**
```powershell
patchwork upgrade Microsoft.VisualStudioCode
```

**Upgrade all available packages:**
```powershell
patchwork upgrade --all
```
