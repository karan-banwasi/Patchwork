## pw

Patchwork is a CLI tool to manage package updates

### Synopsis

Patchwork iterates through packages that have available updates and helps manage them across 9 supported package managers: Homebrew, Winget, Scoop, Chocolatey, npm Global, .NET Global Tools, uv Python Tools, Rust Cargo Crates, and PowerShell PSResource. Automatically detects host platform and queries only relevant managers.

### Options

```
  -h, --help   help for pw
```

### SEE ALSO

* [pw check](pw_check.md)	 - Check for available updates
* [pw cleanup](pw_cleanup.md)	 - Clean up old package versions and download caches
* [pw completion](pw_completion.md)	 - Generate shell completion script
* [pw config](pw_config.md)	 - Manage Patchwork configuration
* [pw doctor](pw_doctor.md)	 - Show system diagnostics and detected package managers
* [pw pin](pw_pin.md)	 - Pin one or more packages to ignore them during check and upgrade
* [pw pinned](pw_pinned.md)	 - List all pinned (ignored) packages
* [pw self-update](pw_self-update.md)	 - Update Patchwork (pw) to the latest release
* [pw unpin](pw_unpin.md)	 - Unpin one or more previously pinned packages
* [pw upgrade](pw_upgrade.md)	 - Upgrade available packages
* [pw version](pw_version.md)	 - Print the version number of Patchwork

