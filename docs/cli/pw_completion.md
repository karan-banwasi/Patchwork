## pw completion

Generate shell completion script

### Synopsis

To load completions in PowerShell:

  pw completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output to your PowerShell $PROFILE:

  pw completion powershell >> $PROFILE

Note: Package ID completions read from the local cache updated by 'pw check'.


```
pw completion [powershell|bash|zsh|fish]
```

### Options

```
  -h, --help   help for completion
```

### SEE ALSO

* [pw](pw.md)	 - Patchwork is a CLI tool to manage package updates

