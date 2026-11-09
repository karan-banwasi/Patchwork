# Security Policy

The Patchwork team takes the security of our software, build pipelines, and users very seriously. We appreciate the responsible disclosure of security vulnerabilities and are committed to addressing valid security concerns promptly.

---

## Supported Versions

Patchwork follows Semantic Versioning (`vMAJOR.MINOR.PATCH`). Security fixes are actively provided exclusively for the latest released version.

| Version | Supported | Notes |
| :--- | :---: | :--- |
| Latest Release (v0.x / v1.x) | :white_check_mark: | Supported with active security updates |
| Older Minor / Patch Releases | :x: | Unsupported; users must upgrade to the latest release |

To verify your current Patchwork version, run:
```bash
pw version
# or
pw --version
```

To update to the latest supported version, run:
```bash
pw self-update
```
or update via your system package manager (`winget upgrade KaranBanwasi.Patchwork`, `brew upgrade patchwork`, or `scoop update patchwork`).

---

## Reporting a Vulnerability

**DO NOT file public GitHub issues or start public discussions for security vulnerabilities.** Public disclosure exposes users to risk before a fix and release are available.

### Preferred Method: GitHub Private Vulnerability Reporting
Please submit all security advisories privately through GitHub:
:lock: **[Open a Private Security Advisory](https://github.com/karan-banwasi/Patchwork/security/advisories/new)**

Using GitHub Private Vulnerability Reporting allows us to securely collaborate on a fix in a private repository fork, coordinate release timelines, and request a CVE identifier through the GitHub Advisory Database.

### Fallback Method: Email
If you are unable to use GitHub's private reporting feature, email the repository maintainer directly:
- **Contact**: Karan Banwasi
- **Email**: `karan.banwasi@gmail.com`
- **Subject**: `[SECURITY] Vulnerability Report: Patchwork - <Brief Description>`

### Information to Include in Your Report
To help us triage and investigate your report efficiently, please include:
1. **Description**: A clear description of the vulnerability, including potential impact and severity (e.g., arbitrary code execution, privilege escalation, attestation bypass, insecure file handling).
2. **Affected Components**: File paths, functions, or package managers involved (e.g., `cmd/selfupdate.go`, `internal/config/config.go`, `internal/pm/runner.go`).
3. **Target Environment**: Operating system (Windows, macOS, Linux), system architecture (`amd64`, `arm64`), and Go version.
4. **Reproduction Steps**: Step-by-step instructions or a minimal Proof of Concept (PoC) demonstrating the vulnerability.
5. **Mitigation / Remediation**: Any suggested fixes, pull requests, or temporary workarounds.

---

## Response SLAs & Disclosure Policy

We follow Coordinated Vulnerability Disclosure (CVD) principles:

- **Initial Acknowledgment**: We aim to acknowledge receipt of your report within **48 hours** (2 business days).
- **Triage & Validation**: We will evaluate and validate the findings, assess CVSS v3.1 severity, and provide an initial response within **5 business days**.
- **Remediation & Updates**: We will provide periodic progress updates at least every **7 to 14 days** while developing and verifying the fix.
- **Coordinated Release**: We target releasing a patched binary and publishing a security advisory within **30 days** of validation (or up to 90 days for complex, multi-system architectural changes).
- **Attribution & Credit**: Security researchers who responsibly report vulnerabilities will be acknowledged in the GitHub Security Advisory and release notes (unless anonymity is requested).

---

## Built-In Security Architecture & Controls

Patchwork incorporates several defense-in-depth measures:

1. **Cryptographic Build Provenance & Attestations**: Release binaries and manifests are attested using GitHub Actions Build Provenance (`actions/attest-build-provenance` / Sigstore). The built-in self-updater (`pw self-update`) and installation script (`install.ps1`) verify universal cryptographic checksums (`SHA256SUMS`) and provenance attestations.
2. **Atomic Configuration Updates**: Configuration changes (`config.json`) are written atomically via temporary files and atomic renaming, preventing file corruption or race conditions.
3. **Safe Process Execution**: CLI commands are executed with explicit argument slices via Go's `exec.Command` / `pm.Runner` abstraction, preventing arbitrary shell interpolation and argument injection.
4. **Automated Supply Chain Scanning**: Every CI workflow run validates Go dependencies against the Go Vulnerability Database using `govulncheck`.
5. **Output Leak Prevention**: Internal packages strictly route output through designated writers, preventing sensitive data or unformatted stream leaks into stdout/stderr.
6. **Zero Telemetry**: Patchwork collects no user telemetry, phone-home metrics, or external analytics, and fully respects the `--offline` flag.
