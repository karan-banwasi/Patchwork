# Windows Code Signing & SmartScreen Architecture

## Overview
When executables are downloaded from the web onto Windows, Windows Defender SmartScreen evaluates their reputation and Authenticode digital signatures. Unsigned executables or binaries signed with unknown/untrusted certificates trigger SmartScreen warnings ("Windows protected your PC: Microsoft Defender SmartScreen prevented an unrecognized app from starting").

This document describes the code signing architecture for Patchwork (`pw.exe` and `pw-arm64.exe`), detailing integration options with Azure Trusted Signing and SignPath OSS.

---

## Authenticode vs Sigstore / GitHub Attestations

Patchwork employs a defense-in-depth supply chain verification strategy:
1. **GitHub Artifact Attestation & Sigstore Cosign**: Verifies cryptographic provenance, commit digest, build runner identity, and transparency logs via Sigstore. (Verified by `install.ps1`, `install.sh`, and `pw self-update`).
2. **PE Authenticode Digital Signing**: Injects an X.509 code signing certificate and timestamp into the Portable Executable (PE) header. This is the **only** signature mechanism recognized by the Windows kernel and Windows Defender SmartScreen.

---

## Critical Release Pipeline Sequencing Invariant

> ⚠️ **IMPORTANT INVARIANT**:
> Authenticode signing modifies the binary itself by populating the PE Certificate Table.
> **Signing MUST occur BEFORE computing `SHA256SUMS` and generating package manifests (Winget, Scoop).**

```
 ┌────────────────┐      ┌─────────────────────────┐      ┌────────────────────┐
 │  go build .    │ ───> │ Authenticode PE Signing │ ───> │ Compute SHA256SUMS │
 └────────────────┘      └─────────────────────────┘      └────────────────────┘
                                                                     │
 ┌────────────────┐      ┌─────────────────────────┐                 │
 │ Publish GitHub │ <─── │ Attest Provenance (OIDC)│ <───────────────┘
 │ Release        │      │ (actions/attest-build)  │
 └────────────────┘      └─────────────────────────┘
```

If signing occurs after `SHA256SUMS` is computed, checksum verification in `install.ps1`, `install.sh`, `pw self-update`, Scoop, and Winget will fail.

---

## Option 1: Azure Trusted Signing (Artifact Signing)

Microsoft Azure Trusted Signing (now called **Azure Artifact Signing**) is a first-party cloud code-signing service that provides publicly trusted Microsoft certificates and immediate SmartScreen reputation without hardware USB tokens.

Patchwork implements Azure Trusted Signing via [scripts/sign-windows.sh](../scripts/sign-windows.sh) using [Jsign](https://github.com/ebourg/jsign) (`--storetype TRUSTEDSIGNING`), allowing `pw.exe` and `pw-arm64.exe` to be signed and RFC 3161 timestamped directly on Linux CI runners (`ubuntu-latest`) in both `.github/workflows/release.yml` and `.goreleaser.yaml` **before** `SHA256SUMS` and Winget manifests are computed.

### Prerequisites (Azure Portal)
1. An Azure Subscription with the `Microsoft.CodeSigning` resource provider registered.
2. An **Artifact Signing (Trusted Signing) Account** with Identity Validation completed and a **Certificate Profile** (Public Trust).
3. An Entra ID App Registration (Service Principal) assigned the **Trusted Signing Certificate Profile Signer** RBAC role on the Certificate Profile / Signing Account.

### Authentication Options
`scripts/sign-windows.sh` automatically selects between:
1. **GitHub Actions OIDC (Recommended, passwordless)**: Add a Federated Credential on the Entra ID App Registration (`Issuer: https://token.actions.githubusercontent.com`, `Subject: repo:karan-banwasi/Patchwork:ref:refs/heads/main` — and/or tag filter — `Audience: api://AzureADTokenExchange`). The `release` job in `.github/workflows/release.yml` already grants `id-token: write`.
2. **Service Principal Client Secret**: Set the `AZURE_CLIENT_SECRET` repository secret if not using OIDC.

### GitHub Secrets & Variables Configuration
Set these in **GitHub Repository Settings -> Secrets and variables -> Actions**:
- `AZURE_TENANT_ID` (Secret or Variable): Entra ID directory tenant ID
- `AZURE_CLIENT_ID` (Secret or Variable): App Registration client ID
- `AZURE_CLIENT_SECRET` (Secret, optional when using OIDC): Client secret value
- `AZURE_TRUSTED_SIGNING_ACCOUNT` (Secret or Variable): Artifact/Trusted Signing account name
- `AZURE_TRUSTED_SIGNING_PROFILE` (Secret or Variable): Certificate profile name
- `AZURE_TRUSTED_SIGNING_ENDPOINT` (Variable, optional): Regional endpoint URL or host (default: `https://eus.codesigning.azure.net/`)
- `REQUIRE_WINDOWS_SIGNING` (Variable, optional): Set to `true` to fail the release build instead of skipping if `AZURE_CLIENT_ID` is unset

### Workflow Integration (`release.yml`)
`.github/workflows/release.yml` passes the `AZURE_*` environment variables into `Build and Create Release` and invokes:
```bash
bash ./scripts/sign-windows.sh pw.exe pw-arm64.exe
```
between building `pw.exe`/`pw-arm64.exe` and generating `SHA256SUMS`.

---

## Option 2: SignPath OSS Foundation

SignPath provides free code signing certificates from a trusted public CA for open-source projects.

### Prerequisites:
1. Apply and receive approval for SignPath Open Source project program.
2. Configure GitHub repository integration and signing policy in the SignPath web dashboard.
3. Configure `SIGNPATH_API_TOKEN` in GitHub repository secrets.

### Workflow Integration (`release.yml`):
```yaml
      - name: Sign Windows Binaries (SignPath)
        if: env.SIGNPATH_API_TOKEN != ''
        uses: signpath/github-action-submit-signing-request@v1.1
        with:
          api-token: ${{ secrets.SIGNPATH_API_TOKEN }}
          organization-id: ${{ secrets.SIGNPATH_ORGANIZATION_ID }}
          project-slug: 'patchwork'
          signing-policy-slug: 'release-signing'
          artifact-configuration-slug: 'windows-binaries'
          wait-for-completion: true
          output-artifact-directory: './signed'
```

---

## Windows Application Control / Smart App Control Blocks

When prebuilt GitHub release binaries (`pw.exe` / `pw-arm64.exe`) are downloaded onto a Windows PC with **Smart App Control**, **Windows Defender Application Control (WDAC)**, or **AppLocker** enforcing code integrity or path restrictions, PowerShell may report:

```text
ResourceUnavailable: Program 'pw.exe' failed to run: An error occurred trying to start process
'...\pw.exe' with working directory '...'. An Application Control policy has blocked this file.
```

### Why This Happens
- **Unsigned PE Binary**: Until a public Authenticode certificate is configured in CI (`AZURE_CLIENT_ID` / `SIGNPATH_API_TOKEN`), release executables carry GitHub/Sigstore attestations and `SHA256SUMS`, not a Microsoft-trusted PE Authenticode signature. Windows 11 Smart App Control blocks unsigned binaries downloaded from the internet.
- **User-Writable Install Location**: Enterprise WDAC/AppLocker policies often disallow executing binaries under `%LOCALAPPDATA%` (`C:\Users\<user>\AppData\Local\Programs\Patchwork\bin`), allowing execution only from trusted system directories such as `$env:ProgramFiles` or from locally trusted publishers.

### Local Remediation Options
1. **Unblock Mark-of-the-Web**: `install.ps1` automatically calls `Unblock-File` on the installed binary.
2. **Self-Sign & Locally Trust the Binary (Administrator PowerShell)**:
   ```powershell
   irm https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.ps1 -OutFile install.ps1
   .\install.ps1 -SelfSign
   ```
   `-SelfSign` creates (or reuses) a `CN=Patchwork Local Code Signing` certificate in `Cert:\LocalMachine\My`, trusts it in `LocalMachine\Root` and `LocalMachine\TrustedPublisher`, and Authenticode-signs `pw.exe`.
3. **Install Under Program Files (Corporate WDAC/AppLocker)**:
   ```powershell
   irm https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.ps1 -OutFile install.ps1
   .\install.ps1 -InstallDir "$env:ProgramFiles\Patchwork\bin" -SelfSign
   ```
4. **Smart App Control Note**: Note that WDAC policies enforced at kernel level still require disabling or adjusting **Windows Security -> App & browser control -> Smart App Control** if local self-signed certificates are excluded by the policy.
