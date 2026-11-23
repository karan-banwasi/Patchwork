## pw import

Import packages and pins from a machine profile

### Synopsis

Read a Patchfile (or JSON profile) from a file or stdin ('-'), restore configured pins, and install missing packages.

```
pw import [file] [flags]
```

### Options

```
      --dry-run          Preview packages and pins that would be imported without modifying the system
  -h, --help             help for import
      --json             Output import results as JSON
  -m, --manager string   Import packages only for a specific package manager
      --skip-existing    Skip packages that are already installed (default true)
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

