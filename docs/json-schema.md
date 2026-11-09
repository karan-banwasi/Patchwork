# JSON Output & Configuration Schemas

Patchwork provides stable, machine-readable JSON interfaces for automation tools, monitoring daemons, and shell status prompts.

All JSON payloads emitted by Patchwork commands adhere to Semantic Versioning and include a top-level `"schema_version"` property.

---

## 1. Schema Versioning Contract

The current schema version across all commands is:
```json
"schema_version": "1.0.0"
```

- **Additive Changes**: New non-breaking fields may be introduced in minor updates (maintaining `1.x.x`).
- **Breaking Changes**: Field renaming or type modifications will increment the major version (`2.0.0`).

---

## 2. Check Command Schema (`pw check --json`)

Emitted by `pw check --json`.

### Schema Definition
| Field | Type | Description |
| :--- | :--- | :--- |
| `schema_version` | string | The schema version contract (`"1.0.0"`). |
| `count` | integer | Total number of updates available across active managers. |
| `pinned_skipped` | integer | Number of discovered updates suppressed because the package was pinned. |
| `updates` | array of objects | List of available package updates. |

Each object in `updates` contains:
| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | string | Human-readable package display name. |
| `id` | string | Canonical package identifier used for upgrades and pinning. |
| `version` | string | Currently installed version. |
| `available_version` | string | New version available from the repository/bucket. |
| `manager` | string | Canonical lowercase name of the package manager (`brew`, `winget`, `scoop`, `choco`, `npm`, `dotnet`, `uv`, `cargo`, `psresource`). |

### Example Payload
```json
{
  "schema_version": "1.0.0",
  "count": 2,
  "pinned_skipped": 1,
  "updates": [
    {
      "name": "Git",
      "id": "Git.Git",
      "version": "2.44.0",
      "available_version": "2.45.0",
      "manager": "winget"
    },
    {
      "name": "ripgrep",
      "id": "ripgrep",
      "version": "14.1.0",
      "available_version": "14.1.1",
      "manager": "cargo"
    }
  ]
}
```

---

## 3. Upgrade Command Schema (`pw upgrade --json`)

Emitted by `pw upgrade --json` (with `--all`, `--dry-run`, or explicit package IDs).

### Schema Definition
| Field | Type | Description |
| :--- | :--- | :--- |
| `schema_version` | string | The schema version contract (`"1.0.0"`). |
| `dry_run` | boolean | `true` if simulated via `--dry-run`, `false` if real upgrade executed. |
| `total` | integer | Total number of packages targeted in this plan. |
| `succeeded` | integer | Number of packages upgraded successfully (or simulated in dry-run). |
| `failed` | integer | Number of packages that failed to upgrade. |
| `skipped` | integer | Number of packages skipped (e.g. pinned or deferred). |
| `results` | array of objects | Granular per-package outcome objects. |

Each object in `results` contains:
| Field | Type | Description |
| :--- | :--- | :--- |
| `package` | object | The package metadata (`name`, `id`, `version`, `available_version`, `manager`). |
| `status` | string | Final state: `"Succeeded"`, `"Failed"`, `"Dry-Run"`, or `"Skipped"`. |
| `duration_ms` | integer | Execution time for this package in milliseconds. |
| `duration` | string | Human-readable duration string (e.g. `"1.24s"`). |
| `error` | string or null | Error message if failed, or `null` if succeeded/dry-run. |

### Example Payload (Execution with Partial Failure)
```json
{
  "schema_version": "1.0.0",
  "dry_run": false,
  "total": 2,
  "succeeded": 1,
  "failed": 1,
  "skipped": 0,
  "results": [
    {
      "package": {
        "name": "curl",
        "id": "curl",
        "version": "8.6.0",
        "available_version": "8.7.1",
        "manager": "scoop"
      },
      "status": "Succeeded",
      "duration_ms": 3410,
      "duration": "3.41s",
      "error": null
    },
    {
      "package": {
        "name": "Node.js",
        "id": "OpenJS.NodeJS",
        "version": "20.11.0",
        "available_version": "20.12.0",
        "manager": "winget"
      },
      "status": "Failed",
      "duration_ms": 1250,
      "duration": "1.25s",
      "error": "exit status 1: installer hash mismatch"
    }
  ]
}
```

---

## 4. Pinned List Schema (`pw pinned --json`)

Emitted by `pw pinned --json`.

### Schema Definition
| Field | Type | Description |
| :--- | :--- | :--- |
| `schema_version` | string | The schema version contract (`"1.0.0"`). |
| `count` | integer | Total number of pinned packages. |
| `pinned_packages` | array of objects | List of pinned package definitions. |

Each object in `pinned_packages` contains:
| Field | Type | Description |
| :--- | :--- | :--- |
| `raw` | string | The raw pin string stored in configuration (e.g. `"Git.Git"` or `"winget:Git.Git"`). |
| `id` | string | The sanitized package ID. |
| `manager` | string (optional) | Scoped package manager name, or omitted/empty if pinned globally across all managers. |

### Example Payload
```json
{
  "schema_version": "1.0.0",
  "count": 2,
  "pinned_packages": [
    {
      "raw": "Git.Git",
      "id": "Git.Git"
    },
    {
      "raw": "scoop:curl",
      "id": "curl",
      "manager": "scoop"
    }
  ]
}
```

---

## 5. Persistent Configuration Schema (`config.json`)

The primary configuration file located at `PATCHWORK_CONFIG` (or default OS path):

### File Structure
```json
{
  "pinned_packages": [
    "Git.Git",
    "scoop:curl",
    "winget:Microsoft.VisualStudioCode"
  ],
  "accept_agreements": true
}
```

| Property | Type | Description |
| :--- | :--- | :--- |
| `pinned_packages` | array of strings | Package IDs to skip during check and upgrade operations. Prefixed with `<manager>:` to scope to a specific package manager, or bare ID to apply globally. |
| `accept_agreements` | boolean (optional) | When `true`, automatically accepts package agreements in supported managers (e.g., Winget). |
