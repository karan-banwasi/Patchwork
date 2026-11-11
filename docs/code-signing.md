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

## Option 1: Azure Trusted Signing (Microsoft ACS)

Microsoft Azure Trusted Signing is a first-party cloud code-signing service that establishes immediate SmartScreen reputation without hardware USB tokens.

### Prerequisites:
1. Azure Subscription with Microsoft.CodeSigning resource provider registered.
2. Trusted Signing Account with Identity Validation completed.
3. Certificate Profile configured for Public Trust.
4. Azure Service Principal configured with `Trusted Signing Certificate Profile Signer` role.

### GitHub Secret Configuration:
- `AZURE_CLIENT_ID`: Service principal app ID
- `AZURE_TENANT_ID`: Azure directory tenant ID
- `AZURE_CLIENT_SECRET`: Service principal secret (or federated OIDC credential)
- `AZURE_TRUSTED_SIGNING_ACCOUNT`: Account name
- `AZURE_TRUSTED_SIGNING_PROFILE`: Certificate profile name

### Workflow Integration (`release.yml`):
```yaml
      - name: Sign Windows Binaries (Azure Trusted Signing)
        if: env.AZURE_CLIENT_ID != ''
        uses: azure/trusted-signing-action@v0.4.0
        with:
          azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}
          azure-client-secret: ${{ secrets.AZURE_CLIENT_SECRET }}
          endpoint: https://eus.codesigning.azure.net/
          trusted-signing-account-name: ${{ secrets.AZURE_TRUSTED_SIGNING_ACCOUNT }}
          certificate-profile-name: ${{ secrets.AZURE_TRUSTED_SIGNING_PROFILE }}
          files: |
            pw.exe
            pw-arm64.exe
          file-digest: SHA256
          timestamp-rfc3161: http://timestamp.acs.microsoft.com
          timestamp-digest: SHA256
```

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
