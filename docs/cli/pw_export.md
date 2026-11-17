## pw export

Export installed packages and pins to a machine profile

### Synopsis

Export installed packages across active package managers and configured pins into a portable Patchfile DSL or JSON profile.

```
pw export [flags]
```

### Options

```
  -h, --help               help for export
      --include-pins       Include configured package pins in the exported profile (default true)
      --include-versions   Include installed package versions in the exported profile
      --json               Output exported profile in JSON format
  -m, --manager string     Export packages from a specific package manager
  -o, --output string      Write exported profile to a file instead of stdout
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

