package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// Helper to generate custom Sigstore test bundles with full control over cryptographic properties
func generateAdversarialBundle(t *testing.T, opts bundleGenOptions) []byte {
	t.Helper()

	// 1. Generate key pair
	var privKey *ecdsa.PrivateKey
	var rsaKey *rsa.PrivateKey
	var pubKey any

	if opts.useRSAKey {
		var err error
		rsaKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("failed to generate rsa key: %v", err)
		}
		pubKey = &rsaKey.PublicKey
	} else {
		var err error
		privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate ecdsa key: %v", err)
		}
		pubKey = &privKey.PublicKey
	}

	// 2. Generate Certificate
	var certDER []byte
	if !opts.omitCert {
		uriList := []*url.URL{}
		if opts.signerURI != "" {
			parsed, err := url.Parse(opts.signerURI)
			if err != nil {
				t.Fatalf("failed to parse signerURI: %v", err)
			}
			uriList = append(uriList, parsed)
		}

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1337),
			Subject: pkix.Name{
				CommonName: opts.commonName,
			},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(1 * time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
			URIs:                  uriList,
		}

		var err error
		if opts.useRSAKey {
			certDER, err = x509.CreateCertificate(rand.Reader, template, template, pubKey, rsaKey)
		} else {
			certDER, err = x509.CreateCertificate(rand.Reader, template, template, pubKey, privKey)
		}
		if err != nil {
			t.Fatalf("failed to create certificate: %v", err)
		}
	}

	// 3. Build in-toto statement payload
	subjects := []inTotoSubject{}
	for _, s := range opts.subjects {
		subjects = append(subjects, inTotoSubject{
			Name:   s.name,
			Digest: s.digests,
		})
	}
	stmt := inTotoStatement{
		Type:          "https://in-toto.io/Statement/v1",
		Subject:       subjects,
		PredicateType: "https://slsa.dev/provenance/v1",
	}

	stmtBytes, err := json.Marshal(stmt)
	if err != nil {
		t.Fatalf("failed to marshal statement: %v", err)
	}

	if opts.tamperPayloadAfterSigning != nil {
		// Used later
	}

	payloadType := opts.payloadType
	if payloadType == "" {
		payloadType = "application/vnd.in-toto+json"
	}

	// 4. DSSE PAE and Sign
	paePrefix := opts.paePrefix
	if paePrefix == "" {
		paePrefix = fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(stmtBytes))
	}
	pae := append([]byte(paePrefix), stmtBytes...)
	paeHash := sha256.Sum256(pae)

	var sigBytes []byte
	if !opts.useRSAKey && privKey != nil {
		if opts.rawP1363Sig {
			r, s, err := ecdsa.Sign(rand.Reader, privKey, paeHash[:])
			if err != nil {
				t.Fatalf("failed to sign: %v", err)
			}
			rBytes := r.Bytes()
			sBytes := s.Bytes()
			sigBytes = make([]byte, 64)
			copy(sigBytes[32-len(rBytes):32], rBytes)
			copy(sigBytes[64-len(sBytes):64], sBytes)
		} else {
			var err error
			sigBytes, err = ecdsa.SignASN1(rand.Reader, privKey, paeHash[:])
			if err != nil {
				t.Fatalf("failed to sign ASN1: %v", err)
			}
		}
	} else if opts.useRSAKey {
		sigBytes = []byte("rsa-dummy-sig")
	}

	if opts.tamperSig {
		if len(sigBytes) > 5 {
			sigBytes[len(sigBytes)-1] ^= 0xFF
			sigBytes[len(sigBytes)-2] ^= 0xFF
		} else {
			sigBytes = []byte("corrupt-signature")
		}
	}
	if opts.truncateSig {
		sigBytes = sigBytes[:len(sigBytes)/2]
	}

	actualPayloadBytes := stmtBytes
	if opts.tamperPayloadAfterSigning != nil {
		actualPayloadBytes = opts.tamperPayloadAfterSigning
	}

	// 5. Construct Sigstore Bundle
	bundle := sigstoreBundle{
		MediaType: "application/vnd.dev.sigstore.bundle+json;version=0.2",
	}

	if !opts.omitCert {
		bundle.VerificationMaterial = &bundleVerificationMat{
			Certificate: &bundleCert{
				RawBytes: base64.StdEncoding.EncodeToString(certDER),
			},
		}
	}

	if opts.corruptCertBase64 {
		bundle.VerificationMaterial = &bundleVerificationMat{
			Certificate: &bundleCert{
				RawBytes: "!!!NotValidBase64!!!",
			},
		}
	}

	if opts.useCertChain {
		bundle.VerificationMaterial = &bundleVerificationMat{
			X509CertificateChain: &bundleCertChain{
				Certificates: []bundleCert{
					{RawBytes: base64.StdEncoding.EncodeToString(certDER)},
				},
			},
		}
	}

	if !opts.omitEnvelope {
		bundle.DSSEEnvelope = &dsseEnvelope{
			Payload:     base64.StdEncoding.EncodeToString(actualPayloadBytes),
			PayloadType: payloadType,
			Signatures: []dsseSignature{
				{
					KeyID: "key-1",
					Sig:   base64.StdEncoding.EncodeToString(sigBytes),
				},
			},
		}
	}

	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal bundle: %v", err)
	}
	return raw
}

type bundleGenOptions struct {
	subjects []struct {
		name    string
		digests map[string]string
	}
	signerURI                 string
	commonName                string
	payloadType               string
	paePrefix                 string
	rawP1363Sig               bool
	tamperSig                 bool
	truncateSig               bool
	tamperPayloadAfterSigning []byte
	useRSAKey                 bool
	omitCert                  bool
	corruptCertBase64         bool
	useCertChain              bool
	omitEnvelope              bool
}

func TestAdversarial_CryptographicBundleAttacks(t *testing.T) {
	validSHA := strings.ToUpper(hex.EncodeToString(sha256.New().Sum([]byte("binary-test-data"))))
	repo := "karan-banwasi/Patchwork"
	pinnedWorkflow := repo + "/.github/workflows/release.yml"
	validURI := "https://github.com/" + pinnedWorkflow + "@refs/tags/v1.0.0"

	defaultSubjects := []struct {
		name    string
		digests map[string]string
	}{
		{
			name:    "pw.exe",
			digests: map[string]string{"sha256": validSHA},
		},
	}

	t.Run("valid ASN1 ecdsa signature passes", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: validURI,
		})
		if err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA); err != nil {
			t.Fatalf("expected valid ASN1 bundle to pass, got: %v", err)
		}
	})

	t.Run("valid raw 64-byte P1363 ecdsa signature passes", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:    defaultSubjects,
			signerURI:   validURI,
			rawP1363Sig: true,
		})
		if err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA); err != nil {
			t.Fatalf("expected valid P1363 bundle to pass, got: %v", err)
		}
	})

	t.Run("valid certificate via X509CertificateChain passes", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:     defaultSubjects,
			signerURI:    validURI,
			useCertChain: true,
		})
		if err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA); err != nil {
			t.Fatalf("expected cert chain bundle to pass, got: %v", err)
		}
	})

	t.Run("tampered signature bits fail verification", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: validURI,
			tamperSig: true,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "cryptographic signature verification failed") {
			t.Fatalf("expected signature verification failure, got: %v", err)
		}
	})

	t.Run("truncated signature fails verification", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:    defaultSubjects,
			signerURI:   validURI,
			truncateSig: true,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "cryptographic signature verification failed") {
			t.Fatalf("expected signature verification failure on truncated sig, got: %v", err)
		}
	})

	t.Run("payload modified after signing fails verification", func(t *testing.T) {
		tamperedStmt := []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[{"name":"pw.exe","digest":{"sha256":"` + validSHA + `"}},{"name":"extra.exe","digest":{"sha256":"abc"}}]}`)
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:                  defaultSubjects,
			signerURI:                 validURI,
			tamperPayloadAfterSigning: tamperedStmt,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "cryptographic signature verification failed") {
			t.Fatalf("expected signature failure on modified payload, got: %v", err)
		}
	})

	t.Run("digest mismatch fails verification", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects: []struct {
				name    string
				digests map[string]string
			}{
				{name: "pw.exe", digests: map[string]string{"sha256": strings.Repeat("B", 64)}},
			},
			signerURI: validURI,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "attestation statement subject digests do not match") {
			t.Fatalf("expected digest mismatch failure, got: %v", err)
		}
	})

	t.Run("statement with multiple subjects passes if one matches", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects: []struct {
				name    string
				digests map[string]string
			}{
				{name: "pw-arm64.exe", digests: map[string]string{"sha256": strings.Repeat("0", 64)}},
				{name: "pw.exe", digests: map[string]string{"sha256": validSHA}},
				{name: "pw-linux-amd64", digests: map[string]string{"sha256": strings.Repeat("1", 64)}},
			},
			signerURI: validURI,
		})
		if err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA); err != nil {
			t.Fatalf("expected multi-subject statement to pass when target matches, got: %v", err)
		}
	})

	t.Run("untrusted repository fails verification", func(t *testing.T) {
		evilURI := "https://github.com/evil-org/Patchwork/.github/workflows/release.yml@refs/tags/v1.0.0"
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: evilURI,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "does not match expected signer workflow") {
			t.Fatalf("expected untrusted repo failure, got: %v", err)
		}
	})

	t.Run("untrusted repository with matching suffix (attacker-karan-banwasi) fails verification", func(t *testing.T) {
		evilURI := "https://github.com/attacker-karan-banwasi/Patchwork/.github/workflows/release.yml@refs/tags/v1.0.0"
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: evilURI,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil {
			t.Fatalf("SECURITY VULNERABILITY: bundle from untrusted repository 'attacker-karan-banwasi/Patchwork' was accepted because of unanchored substring match!")
		}
	})

	t.Run("untrusted repository spoofing workflow in ref fails verification", func(t *testing.T) {
		// Attacker owns totally different repo, but names a branch after the target workflow
		evilURI := "https://github.com/attacker/malware/.github/workflows/attack.yml@refs/heads/karan-banwasi/Patchwork/.github/workflows/release.yml"
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: evilURI,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil {
			t.Fatalf("SECURITY VULNERABILITY: bundle from untrusted repository 'attacker/malware' was accepted because pinnedWorkflow was matched in the ref part of the URI!")
		}
	})

	t.Run("wrong workflow in same repository fails verification", func(t *testing.T) {
		wrongURI := "https://github.com/karan-banwasi/Patchwork/.github/workflows/pr-build.yml@refs/pull/42/merge"
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: wrongURI,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "does not match expected signer workflow") {
			t.Fatalf("expected wrong workflow failure, got: %v", err)
		}
	})

	t.Run("RSA key in certificate rejected", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: validURI,
			useRSAKey: true,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "unsupported public key type") {
			t.Fatalf("expected RSA rejection, got: %v", err)
		}
	})

	t.Run("missing certificate in bundle fails closed", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:  defaultSubjects,
			signerURI: validURI,
			omitCert:  true,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "missing certificate") {
			t.Fatalf("expected missing certificate error, got: %v", err)
		}
	})

	t.Run("corrupt base64 certificate fails closed", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:          defaultSubjects,
			signerURI:         validURI,
			corruptCertBase64: true,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "missing certificate") {
			t.Fatalf("expected corrupt cert error, got: %v", err)
		}
	})

	t.Run("missing envelope fails closed", func(t *testing.T) {
		bundle := generateAdversarialBundle(t, bundleGenOptions{
			subjects:     defaultSubjects,
			signerURI:    validURI,
			omitEnvelope: true,
		})
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "missing DSSE envelope") {
			t.Fatalf("expected missing envelope error, got: %v", err)
		}
	})

	t.Run("corrupt bundle JSON fails closed", func(t *testing.T) {
		err := verifyAttestationBundle([]byte("{ corrupt json: ["), repo, pinnedWorkflow, validSHA)
		if err == nil || !strings.Contains(err.Error(), "failed to parse sigstore bundle") {
			t.Fatalf("expected bundle json parse error, got: %v", err)
		}
	})
}

func TestAdversarial_CLICommands_CorruptConfigFailClosed(t *testing.T) {
	mockUpdates := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", Version: "2.40.0", AvailableVersion: "2.41.0", ManagerName: "winget"},
	}

	setupCorruptEnv := func(t *testing.T, content []byte) (string, func()) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "config.json")
		if err := os.WriteFile(cfgPath, content, 0600); err != nil {
			t.Fatalf("failed to write test config: %v", err)
		}
		t.Setenv("PATCHWORK_CONFIG", cfgPath)

		mockMgr := &mockCheckManager{
			name:    "winget",
			updates: mockUpdates,
		}
		registryOverride = pm.NewRegistry(mockMgr)
		cleanup := func() {
			registryOverride = nil
		}
		return cfgPath, cleanup
	}

	corruptContents := map[string][]byte{
		"truncated JSON": []byte(`{"pinned_packages": ["git"`),
		"empty file":     []byte(``),
		"garbage bytes":  []byte(`{ corrupt: invalid json syntax }`),
		"type mismatch":  []byte(`{"pinned_packages": 9999}`),
	}

	for desc, content := range corruptContents {
		t.Run("pw check fails closed on "+desc, func(t *testing.T) {
			_, cleanup := setupCorruptEnv(t, content)
			defer cleanup()

			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"check", "--no-refresh"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected 'pw check' to fail on %s, got nil", desc)
			}
			if !strings.Contains(err.Error(), "cannot verify pinned packages") {
				t.Errorf("unexpected error on check: %v", err)
			}

			// Bypassed with --include-pinned
			buf.Reset()
			cmdBypass := NewRootCmd()
			cmdBypass.SetOut(buf)
			cmdBypass.SetErr(buf)
			cmdBypass.SetArgs([]string{"check", "--no-refresh", "--include-pinned"})
			if err := cmdBypass.Execute(); err != nil {
				t.Fatalf("expected 'pw check --include-pinned' to succeed, got: %v", err)
			}
		})

		t.Run("pw upgrade --all fails closed on "+desc, func(t *testing.T) {
			_, cleanup := setupCorruptEnv(t, content)
			defer cleanup()

			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected 'pw upgrade --all' to fail on %s, got nil", desc)
			}
			if !strings.Contains(err.Error(), "cannot verify pinned packages") {
				t.Errorf("unexpected error on upgrade --all: %v", err)
			}

			// Overridden with --force
			buf.Reset()
			cmdForce := NewRootCmd()
			cmdForce.SetOut(buf)
			cmdForce.SetErr(buf)
			cmdForce.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh", "--force"})
			if err := cmdForce.Execute(); err != nil {
				t.Fatalf("expected 'pw upgrade --all --force' to bypass on %s, got: %v", desc, err)
			}

			// Overridden with --include-pinned
			buf.Reset()
			cmdInc := NewRootCmd()
			cmdInc.SetOut(buf)
			cmdInc.SetErr(buf)
			cmdInc.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh", "--include-pinned"})
			if err := cmdInc.Execute(); err != nil {
				t.Fatalf("expected 'pw upgrade --all --include-pinned' to bypass on %s, got: %v", desc, err)
			}
		})

		t.Run("pw upgrade <pkg> fails closed on "+desc, func(t *testing.T) {
			_, cleanup := setupCorruptEnv(t, content)
			defer cleanup()

			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"upgrade", "Git.Git", "--dry-run", "-m", "winget"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected 'pw upgrade Git.Git' to fail on %s, got nil", desc)
			}
			if !strings.Contains(err.Error(), "cannot verify pinned packages") {
				t.Errorf("unexpected error on upgrade <pkg>: %v", err)
			}

			// Overridden with --force
			buf.Reset()
			cmdForce := NewRootCmd()
			cmdForce.SetOut(buf)
			cmdForce.SetErr(buf)
			cmdForce.SetArgs([]string{"upgrade", "Git.Git", "--dry-run", "-m", "winget", "--force"})
			if err := cmdForce.Execute(); err != nil {
				t.Fatalf("expected 'pw upgrade Git.Git --force' to bypass on %s, got: %v", desc, err)
			}
		})

		t.Run("pw pin fails closed and preserves corrupt file on "+desc, func(t *testing.T) {
			cfgPath, cleanup := setupCorruptEnv(t, content)
			defer cleanup()

			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"pin", "SomePackage"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected 'pw pin' to fail on %s, got nil", desc)
			}
			if !strings.Contains(err.Error(), "failed to load config") {
				t.Errorf("unexpected error on pin: %v", err)
			}

			// Verify corrupt config was NOT overwritten or destroyed
			afterBytes, readErr := os.ReadFile(cfgPath)
			if readErr != nil {
				t.Fatalf("failed to read config file after pin failure: %v", readErr)
			}
			if !bytes.Equal(afterBytes, content) {
				t.Fatalf("SECURITY VIOLATION: corrupt config file was modified by failed 'pw pin'!\nBefore: %q\nAfter: %q", string(content), string(afterBytes))
			}
		})

		t.Run("pw unpin fails closed on "+desc, func(t *testing.T) {
			_, cleanup := setupCorruptEnv(t, content)
			defer cleanup()

			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"unpin", "SomePackage"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected 'pw unpin' to fail on %s, got nil", desc)
			}
			if !strings.Contains(err.Error(), "failed to load config") {
				t.Errorf("unexpected error on unpin: %v", err)
			}
		})

		t.Run("pw pinned fails closed on "+desc, func(t *testing.T) {
			_, cleanup := setupCorruptEnv(t, content)
			defer cleanup()

			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"pinned"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected 'pw pinned' to fail on %s, got nil", desc)
			}
			if !strings.Contains(err.Error(), "failed to load config") {
				t.Errorf("unexpected error on pinned: %v", err)
			}
		})
	}
}

func TestAdversarial_SelfUpdate_AttestationAPIEdgeCases(t *testing.T) {
	baseURL := "https://mock.github.test"
	payload := []byte("binary-test-payload")
	sum := sha256.Sum256(payload)
	validHash := strings.ToUpper(hex.EncodeToString(sum[:]))

	t.Run("REST attestation 404 with requireAttestation fails immediately", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.Contains(r.URL.Path, "attestations"):
					return makeResponse(http.StatusNotFound, []byte(`{"message":"Not Found"}`)), nil
				case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
					return makeResponse(http.StatusOK, []byte(fmt.Sprintf("%s  pw.exe\n", strings.ToLower(validHash)))), nil
				case strings.HasSuffix(r.URL.Path, "pw.exe"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v9.9.9",
						Assets: []ghAsset{
							{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
							{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/SHA256SUMS"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old-binary"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{
			requireAttestation: true,
		}, &out)
		if err == nil || !strings.Contains(err.Error(), "refusing to update without authentic provenance") {
			t.Fatalf("expected fail-closed error on 404 attestation, got: %v", err)
		}
		// Confirm binary was not overwritten
		current, _ := os.ReadFile(exePath)
		if string(current) != "old-binary" {
			t.Errorf("binary was modified on failed attestation!")
		}
	})

	t.Run("REST attestation returns empty array fails immediately", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.Contains(r.URL.Path, "attestations"):
					return makeResponse(http.StatusOK, []byte(`{"attestations":[]}`)), nil
				case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
					return makeResponse(http.StatusOK, []byte(fmt.Sprintf("%s  pw.exe\n", strings.ToLower(validHash)))), nil
				case strings.HasSuffix(r.URL.Path, "pw.exe"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v9.9.9",
						Assets: []ghAsset{
							{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
							{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/SHA256SUMS"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old-binary"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{
			requireAttestation: true,
		}, &out)
		if err == nil || !strings.Contains(err.Error(), "zero attestations found") {
			t.Fatalf("expected error on empty attestations array, got: %v", err)
		}
		current, _ := os.ReadFile(exePath)
		if string(current) != "old-binary" {
			t.Errorf("binary was modified on empty attestations array!")
		}
	})
}
