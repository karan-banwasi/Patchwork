package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func makeResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func createTestSigstoreBundle(t *testing.T, targetDigestHex, workflowURL string, corruptSig bool) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	u, err := url.Parse(workflowURL)
	if err != nil {
		t.Fatalf("failed to parse workflow URL: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "GitHub Actions",
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Hour),
		URIs:      []*url.URL{u},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	stmt := inTotoStatement{
		Type: "https://in-toto.io/Statement/v1",
		Subject: []inTotoSubject{
			{
				Name: "pw.exe",
				Digest: map[string]string{
					"sha256": strings.ToLower(targetDigestHex),
				},
			},
		},
		PredicateType: "https://slsa.dev/provenance/v1",
	}
	payloadBytes, err := json.Marshal(stmt)
	if err != nil {
		t.Fatalf("failed to marshal in-toto statement: %v", err)
	}

	payloadType := "application/vnd.in-toto+json"
	pae := []byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(payloadBytes)))
	pae = append(pae, payloadBytes...)
	paeHash := sha256.Sum256(pae)

	sig, err := ecdsa.SignASN1(rand.Reader, priv, paeHash[:])
	if err != nil {
		t.Fatalf("failed to sign DSSE PAE: %v", err)
	}

	if corruptSig {
		sig[len(sig)-1] ^= 0xFF
	}

	bundle := sigstoreBundle{
		MediaType: "application/vnd.dev.sigstore.bundle.v0.3+json",
		VerificationMaterial: &bundleVerificationMat{
			Certificate: &bundleCert{
				RawBytes: base64.StdEncoding.EncodeToString(certDER),
			},
		},
		DSSEEnvelope: &dsseEnvelope{
			Payload:     base64.StdEncoding.EncodeToString(payloadBytes),
			PayloadType: payloadType,
			Signatures: []dsseSignature{
				{
					KeyID: "test-key-id",
					Sig:   base64.StdEncoding.EncodeToString(sig),
				},
			},
		},
	}

	bundleBytes, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal bundle: %v", err)
	}
	return bundleBytes
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"dev", "v1.0.0", true},
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.5", "v1.1.0", true},
		{"v1.2.0", "v2.0.0", true},
		{"v1.0.5", "v1.0.5", false},
		{"v1.0.6", "v1.0.5", false},
	}

	for _, tc := range tests {
		got := isNewerVersion(tc.current, tc.latest)
		if got != tc.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestExtractExpectedSHA256(t *testing.T) {
	sampleYAML := `PackageIdentifier: KaranBanwasi.Patchwork
PackageVersion: 0.0.22
InstallerType: portable
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v0.0.22/pw.exe
    InstallerSha256: AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899
  - Architecture: arm64
    InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v0.0.22/pw-arm64.exe
    InstallerSha256: 11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF
ManifestType: installer
`
	gotX64 := extractExpectedSHA256(sampleYAML, "x64")
	if gotX64 != "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899" {
		t.Errorf("unexpected x64 sha: %q", gotX64)
	}
	gotArm64 := extractExpectedSHA256(sampleYAML, "arm64")
	if gotArm64 != "11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF" {
		t.Errorf("unexpected arm64 sha: %q", gotArm64)
	}
}

func TestDetectManagedInstall(t *testing.T) {
	wingetExe := `C:\Users\user\AppData\Local\Microsoft\WinGet\Packages\KaranBanwasi.Patchwork_Microsoft.Winget.Source_8wekyb3d8bbwe\pw.exe`
	if err := detectManagedInstall(wingetExe); err == nil || !strings.Contains(err.Error(), "winget upgrade") {
		t.Errorf("expected winget managed install error, got: %v", err)
	}

	scoopExe := `C:\Users\user\scoop\apps\patchwork\current\pw.exe`
	if err := detectManagedInstall(scoopExe); err == nil || !strings.Contains(err.Error(), "scoop update") {
		t.Errorf("expected scoop managed install error, got: %v", err)
	}

	brewExe := `/opt/homebrew/Cellar/patchwork/1.0.0/bin/pw`
	if err := detectManagedInstall(brewExe); err == nil || !strings.Contains(err.Error(), "brew upgrade") {
		t.Errorf("expected brew managed install error, got: %v", err)
	}

	standaloneExe := `C:\Users\user\AppData\Local\Programs\Patchwork\bin\pw.exe`
	if err := detectManagedInstall(standaloneExe); err != nil {
		t.Errorf("expected standalone path to succeed, got: %v", err)
	}
}

func TestPerformSelfUpdate_ChecksumVerification(t *testing.T) {
	payload := []byte("new-patchwork-binary-contents")
	sum := sha256.Sum256(payload)
	validHash := strings.ToUpper(hex.EncodeToString(sum[:]))
	wrongHash := strings.Repeat("0", 64)
	baseURL := "https://mock.github.test"

	t.Run("manifest missing fails closed", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				rel := ghRelease{
					TagName: "v9.9.9",
					Assets: []ghAsset{
						{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
					},
				}
				b, _ := json.Marshal(rel)
				return makeResponse(http.StatusOK, b), nil
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "no checksum manifest") {
			t.Fatalf("expected missing manifest error, got: %v", err)
		}
	})

	t.Run("unparseable manifest fails closed", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "installer.yaml") {
					return makeResponse(http.StatusOK, []byte("not a valid manifest with hashes")), nil
				}
				rel := ghRelease{
					TagName: "v9.9.9",
					Assets: []ghAsset{
						{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
						{Name: "KaranBanwasi.Patchwork.installer.yaml", BrowserDownloadURL: baseURL + "/installer.yaml"},
					},
				}
				b, _ := json.Marshal(rel)
				return makeResponse(http.StatusOK, b), nil
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "could not determine expected SHA256") {
			t.Fatalf("expected unparseable manifest error, got: %v", err)
		}
	})

	t.Run("hash mismatch fails closed and leaves existing binary intact", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, "installer.yaml"):
					manifest := fmt.Sprintf("Installers:\n  - Architecture: x64\n    InstallerSha256: %s\n", wrongHash)
					return makeResponse(http.StatusOK, []byte(manifest)), nil
				case strings.HasSuffix(r.URL.Path, "pw.exe"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v9.9.9",
						Assets: []ghAsset{
							{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
							{Name: "KaranBanwasi.Patchwork.installer.yaml", BrowserDownloadURL: baseURL + "/installer.yaml"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "SHA256 checksum mismatch") {
			t.Fatalf("expected hash mismatch error, got: %v", err)
		}
		gotBytes, _ := os.ReadFile(exePath)
		if string(gotBytes) != "old" {
			t.Errorf("expected original binary to remain untouched, got %q", string(gotBytes))
		}
	})

	t.Run("attestation missing fails closed", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.Contains(r.URL.Path, "attestations"):
					return makeResponse(http.StatusNotFound, []byte(`{"message":"Not Found"}`)), nil
				case strings.HasSuffix(r.URL.Path, "installer.yaml"):
					manifest := fmt.Sprintf("Installers:\n  - Architecture: x64\n    InstallerSha256: %s\n", validHash)
					return makeResponse(http.StatusOK, []byte(manifest)), nil
				case strings.HasSuffix(r.URL.Path, "pw.exe"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v9.9.9",
						Assets: []ghAsset{
							{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
							{Name: "KaranBanwasi.Patchwork.installer.yaml", BrowserDownloadURL: baseURL + "/installer.yaml"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{skipAttestation: false}, &out)
		if err == nil || !strings.Contains(err.Error(), "provenance attestation") {
			t.Fatalf("expected attestation failure error, got: %v", err)
		}
	})

	t.Run("valid hash and valid attestation replaces binary and cleans up stale files", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.Contains(r.URL.Path, "attestations"):
					validBundle := createTestSigstoreBundle(t, validHash, "https://github.com/karan-banwasi/Patchwork/.github/workflows/release.yml@refs/tags/v9.9.9", false)
					attResp := ghAttestationsResponse{
						Attestations: []struct {
							Bundle json.RawMessage `json:"bundle"`
						}{
							{Bundle: validBundle},
						},
					}
					b, _ := json.Marshal(attResp)
					return makeResponse(http.StatusOK, b), nil
				case strings.HasSuffix(r.URL.Path, "installer.yaml"):
					manifest := fmt.Sprintf("Installers:\n  - Architecture: x64\n    InstallerSha256: %s\n", validHash)
					return makeResponse(http.StatusOK, []byte(manifest)), nil
				case strings.HasSuffix(r.URL.Path, "pw.exe"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v9.9.9",
						Assets: []ghAsset{
							{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
							{Name: "KaranBanwasi.Patchwork.installer.yaml", BrowserDownloadURL: baseURL + "/installer.yaml"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		staleTmp := filepath.Join(tmpDir, "pw-update-stale.exe")
		_ = os.WriteFile(exePath, []byte("old"), 0755)
		_ = os.WriteFile(staleTmp, []byte("stale"), 0600)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{skipAttestation: false}, &out)
		if err != nil {
			t.Fatalf("expected self-update to succeed, got: %v", err)
		}
		if _, err := os.Stat(staleTmp); !os.IsNotExist(err) {
			t.Errorf("expected stale update file to be cleaned up")
		}
		gotBytes, _ := os.ReadFile(exePath)
		if !bytes.Equal(gotBytes, payload) {
			t.Errorf("expected updated binary content, got %q", string(gotBytes))
		}
	})
}

func TestResolveBinaryAsset(t *testing.T) {
	tests := []struct {
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{"windows", "amd64", "pw.exe", false},
		{"windows", "arm64", "pw-arm64.exe", false},
		{"darwin", "arm64", "pw-darwin-arm64", false},
		{"darwin", "amd64", "pw-darwin-amd64", false},
		{"linux", "amd64", "pw-linux-amd64", false},
		{"linux", "arm64", "pw-linux-arm64", false},
		{"windows", "386", "", true},
		{"darwin", "386", "", true},
		{"linux", "mips", "", true},
		{"freebsd", "amd64", "", true},
		{"openbsd", "arm64", "", true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%s", tc.goos, tc.goarch), func(t *testing.T) {
			got, err := resolveBinaryAsset(tc.goos, tc.goarch)
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolveBinaryAsset(%q, %q) error = %v, wantErr = %v", tc.goos, tc.goarch, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("resolveBinaryAsset(%q, %q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestParseSHA256Sums(t *testing.T) {
	content := `# SHA256SUMS for release v1.0.0
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  pw.exe
01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b *pw-arm64.exe
A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2   ./pw-darwin-arm64
11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff	.\pw-darwin-amd64
cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe  dir/sub/pw-linux-amd64
deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  pw-linux-arm64
invalid-short-hash pw-invalid
`
	sums := parseSHA256Sums(content)

	expected := map[string]string{
		"pw.exe":          "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855",
		"pw-arm64.exe":    "01BA4719C80B6FE911B091A7C05124B64EEECE964E09C058EF8F9805DACA546B",
		"pw-darwin-arm64": "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2",
		"pw-darwin-amd64": "11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF",
		"pw-linux-amd64":  "CAFEBABECAFEBABECAFEBABECAFEBABECAFEBABECAFEBABECAFEBABECAFEBABE",
		"pw-linux-arm64":  "DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF",
	}

	for fname, wantHash := range expected {
		gotHash, ok := sums[fname]
		if !ok {
			t.Errorf("missing filename in sums: %s", fname)
			continue
		}
		if gotHash != wantHash {
			t.Errorf("hash for %s = %s, want %s", fname, gotHash, wantHash)
		}
	}

	if _, ok := sums["pw-invalid"]; ok {
		t.Errorf("expected invalid hash line to be excluded")
	}
}

func TestCleanupStaleUpdateFiles(t *testing.T) {
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "pw.exe")
	if err := os.WriteFile(exePath, []byte("active-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	staleFiles := []string{
		exePath + ".old",
		filepath.Join(tmpDir, "pw-update-1234.exe"),
		filepath.Join(tmpDir, "pw-update-5678"),
	}
	keptFiles := []string{
		exePath,
		filepath.Join(tmpDir, "pw"),
		filepath.Join(tmpDir, "unrelated.txt"),
	}

	for _, f := range staleFiles {
		if err := os.WriteFile(f, []byte("stale"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range keptFiles {
		if f == exePath {
			continue
		}
		if err := os.WriteFile(f, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	cleanupStaleUpdateFiles(tmpDir, exePath)

	for _, f := range staleFiles {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected stale file %s to be deleted, but it exists", f)
		}
	}
	for _, f := range keptFiles {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected kept file %s to exist, error: %v", f, err)
		}
	}
}

func TestPerformSelfUpdate_CrossPlatform(t *testing.T) {
	targets := []struct {
		goos      string
		goarch    string
		assetName string
	}{
		{"windows", "amd64", "pw.exe"},
		{"windows", "arm64", "pw-arm64.exe"},
		{"darwin", "arm64", "pw-darwin-arm64"},
		{"darwin", "amd64", "pw-darwin-amd64"},
		{"linux", "amd64", "pw-linux-amd64"},
		{"linux", "arm64", "pw-linux-arm64"},
	}

	for _, target := range targets {
		t.Run(fmt.Sprintf("%s_%s", target.goos, target.goarch), func(t *testing.T) {
			payload := []byte(fmt.Sprintf("binary-for-%s-%s", target.goos, target.goarch))
			sum := sha256.Sum256(payload)
			hashHex := hex.EncodeToString(sum[:])
			baseURL := "https://mock.github.test"

			sumsContent := fmt.Sprintf("%s  %s\n", hashHex, target.assetName)

			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch {
					case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
						return makeResponse(http.StatusOK, []byte(sumsContent)), nil
					case strings.HasSuffix(r.URL.Path, target.assetName):
						return makeResponse(http.StatusOK, payload), nil
					default:
						rel := ghRelease{
							TagName: "v2.0.0",
							Assets: []ghAsset{
								{Name: target.assetName, BrowserDownloadURL: baseURL + "/" + target.assetName},
								{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/SHA256SUMS"},
							},
						}
						b, _ := json.Marshal(rel)
						return makeResponse(http.StatusOK, b), nil
					}
				}),
			}

			tmpDir := t.TempDir()
			exePath := filepath.Join(tmpDir, target.assetName)
			_ = os.WriteFile(exePath, []byte("old-binary"), 0755)

			var out bytes.Buffer
			err := performSelfUpdate(
				context.Background(),
				client,
				baseURL,
				"v1.0.0",
				target.goos,
				target.goarch,
				exePath,
				selfUpdateOptions{skipAttestation: true},
				&out,
			)
			if err != nil {
				t.Fatalf("performSelfUpdate failed: %v", err)
			}

			gotBytes, err := os.ReadFile(exePath)
			if err != nil {
				t.Fatalf("failed to read updated file: %v", err)
			}
			if !bytes.Equal(gotBytes, payload) {
				t.Fatalf("binary content mismatch: got %q, want %q", string(gotBytes), string(payload))
			}
		})
	}
}

func TestPerformSelfUpdate_SHA256SUMS_Fallback(t *testing.T) {
	baseURL := "https://mock.github.test"
	payload := []byte("binary-content")
	sum := sha256.Sum256(payload)
	validHash := strings.ToUpper(hex.EncodeToString(sum[:]))

	t.Run("windows falls back to winget yaml when SHA256SUMS is missing", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, "installer.yaml"):
					manifest := fmt.Sprintf("Installers:\n  - Architecture: x64\n    InstallerSha256: %s\n", validHash)
					return makeResponse(http.StatusOK, []byte(manifest)), nil
				case strings.HasSuffix(r.URL.Path, "pw.exe"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v2.0.0",
						Assets: []ghAsset{
							{Name: "pw.exe", BrowserDownloadURL: baseURL + "/pw.exe"},
							{Name: "KaranBanwasi.Patchwork.installer.yaml", BrowserDownloadURL: baseURL + "/installer.yaml"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw.exe")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err != nil {
			t.Fatalf("expected windows fallback to succeed, got: %v", err)
		}
	})

	t.Run("non-windows fails closed when SHA256SUMS is missing even if winget yaml exists", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, "installer.yaml"):
					manifest := fmt.Sprintf("Installers:\n  - Architecture: x64\n    InstallerSha256: %s\n", validHash)
					return makeResponse(http.StatusOK, []byte(manifest)), nil
				case strings.HasSuffix(r.URL.Path, "pw-darwin-arm64"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v2.0.0",
						Assets: []ghAsset{
							{Name: "pw-darwin-arm64", BrowserDownloadURL: baseURL + "/pw-darwin-arm64"},
							{Name: "KaranBanwasi.Patchwork.installer.yaml", BrowserDownloadURL: baseURL + "/installer.yaml"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw-darwin-arm64")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "darwin", "arm64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "no SHA256SUMS checksum manifest") {
			t.Fatalf("expected darwin without SHA256SUMS to fail closed, got: %v", err)
		}
	})

	t.Run("SHA256SUMS present but target asset missing fails closed", func(t *testing.T) {
		sumsContent := fmt.Sprintf("%s  other-binary\n", validHash)
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
					return makeResponse(http.StatusOK, []byte(sumsContent)), nil
				case strings.HasSuffix(r.URL.Path, "pw-linux-amd64"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v2.0.0",
						Assets: []ghAsset{
							{Name: "pw-linux-amd64", BrowserDownloadURL: baseURL + "/pw-linux-amd64"},
							{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/SHA256SUMS"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw-linux-amd64")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "linux", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "does not contain checksum") {
			t.Fatalf("expected missing target in SHA256SUMS to fail closed, got: %v", err)
		}
	})

	t.Run("SHA256SUMS hash mismatch fails closed and leaves binary intact", func(t *testing.T) {
		wrongHash := strings.Repeat("a", 64)
		sumsContent := fmt.Sprintf("%s  pw-linux-amd64\n", wrongHash)
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
					return makeResponse(http.StatusOK, []byte(sumsContent)), nil
				case strings.HasSuffix(r.URL.Path, "pw-linux-amd64"):
					return makeResponse(http.StatusOK, payload), nil
				default:
					rel := ghRelease{
						TagName: "v2.0.0",
						Assets: []ghAsset{
							{Name: "pw-linux-amd64", BrowserDownloadURL: baseURL + "/pw-linux-amd64"},
							{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/SHA256SUMS"},
						},
					}
					b, _ := json.Marshal(rel)
					return makeResponse(http.StatusOK, b), nil
				}
			}),
		}

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw-linux-amd64")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "linux", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "SHA256 checksum mismatch") {
			t.Fatalf("expected checksum mismatch error, got: %v", err)
		}
		gotBytes, _ := os.ReadFile(exePath)
		if string(gotBytes) != "old" {
			t.Errorf("expected original binary to remain intact, got: %q", string(gotBytes))
		}
	})
}

func TestSelfUpdate_AttestationFlags_MutuallyExclusive(t *testing.T) {
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "pw.exe")
	_ = os.WriteFile(exePath, []byte("old"), 0755)

	var out bytes.Buffer
	err := performSelfUpdate(context.Background(), http.DefaultClient, "https://api.github.test", "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{
		skipAttestation:    true,
		requireAttestation: true,
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "cannot specify both --skip-attestation and --require-attestation") {
		t.Fatalf("expected mutual exclusivity error, got: %v", err)
	}
}

func TestSelfUpdate_CryptographicBundleVerification(t *testing.T) {
	payload := []byte("cryptographic-test-payload")
	sum := sha256.Sum256(payload)
	actualSHA := strings.ToUpper(hex.EncodeToString(sum[:]))
	repo := "karan-banwasi/Patchwork"
	pinnedWorkflow := repo + "/.github/workflows/release.yml"

	t.Run("valid bundle passes verification", func(t *testing.T) {
		bundle := createTestSigstoreBundle(t, actualSHA, "https://github.com/"+pinnedWorkflow+"@refs/tags/v1.0.0", false)
		if err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA); err != nil {
			t.Fatalf("expected valid bundle verification to pass, got: %v", err)
		}
	})

	t.Run("tampered signature fails closed", func(t *testing.T) {
		bundle := createTestSigstoreBundle(t, actualSHA, "https://github.com/"+pinnedWorkflow+"@refs/tags/v1.0.0", true)
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA)
		if err == nil || !strings.Contains(err.Error(), "cryptographic signature verification failed") {
			t.Fatalf("expected cryptographic signature failure, got: %v", err)
		}
	})

	t.Run("digest mismatch fails closed", func(t *testing.T) {
		differentHash := strings.Repeat("A", 64)
		bundle := createTestSigstoreBundle(t, differentHash, "https://github.com/"+pinnedWorkflow+"@refs/tags/v1.0.0", false)
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA)
		if err == nil || !strings.Contains(err.Error(), "attestation statement subject digests do not match") {
			t.Fatalf("expected digest mismatch error, got: %v", err)
		}
	})

	t.Run("wrong signer workflow fails closed", func(t *testing.T) {
		wrongWorkflow := repo + "/.github/workflows/pr-check.yml"
		bundle := createTestSigstoreBundle(t, actualSHA, "https://github.com/"+wrongWorkflow+"@refs/tags/v1.0.0", false)
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA)
		if err == nil || !strings.Contains(err.Error(), "signing certificate does not match expected signer workflow") {
			t.Fatalf("expected wrong signer workflow error, got: %v", err)
		}
	})

	t.Run("untrusted repository fails closed", func(t *testing.T) {
		attackerWorkflow := "attacker/Patchwork/.github/workflows/release.yml"
		bundle := createTestSigstoreBundle(t, actualSHA, "https://github.com/"+attackerWorkflow+"@refs/tags/v1.0.0", false)
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA)
		if err == nil || !strings.Contains(err.Error(), "signing certificate does not match expected signer workflow") {
			t.Fatalf("expected untrusted repository error, got: %v", err)
		}
	})

	t.Run("untrusted repository with matching suffix fails closed", func(t *testing.T) {
		attackerWorkflow := "attacker-karan-banwasi/Patchwork/.github/workflows/release.yml"
		bundle := createTestSigstoreBundle(t, actualSHA, "https://github.com/"+attackerWorkflow+"@refs/tags/v1.0.0", false)
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA)
		if err == nil || !strings.Contains(err.Error(), "signing certificate does not match expected signer workflow") {
			t.Fatalf("expected untrusted repository suffix failure, got: %v", err)
		}
	})

	t.Run("untrusted repository spoofing workflow in ref fails closed", func(t *testing.T) {
		evilURI := "https://github.com/attacker/malware/.github/workflows/attack.yml@refs/heads/karan-banwasi/Patchwork/.github/workflows/release.yml"
		bundle := createTestSigstoreBundle(t, actualSHA, evilURI, false)
		err := verifyAttestationBundle(bundle, repo, pinnedWorkflow, actualSHA)
		if err == nil || !strings.Contains(err.Error(), "signing certificate does not match expected signer workflow") {
			t.Fatalf("expected untrusted repository ref spoof failure, got: %v", err)
		}
	})
}

func TestSelfUpdate_RequireAttestation_FailsWhenMissing(t *testing.T) {
	baseURL := "https://mock.github.test"
	payload := []byte("binary-payload")
	sum := sha256.Sum256(payload)
	validHash := strings.ToUpper(hex.EncodeToString(sum[:]))

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(r.URL.Path, "attestations"):
				return makeResponse(http.StatusNotFound, []byte(`{"message":"Not Found"}`)), nil
			case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
				manifest := fmt.Sprintf("%s  pw.exe\n", strings.ToLower(validHash))
				return makeResponse(http.StatusOK, []byte(manifest)), nil
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
	_ = os.WriteFile(exePath, []byte("old"), 0755)

	var out bytes.Buffer
	err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", exePath, selfUpdateOptions{
		requireAttestation: true,
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "provenance attestation verification gate failed") {
		t.Fatalf("expected requireAttestation failure on 404, got: %v", err)
	}
}

func newTestGitHubClient(handler http.Handler) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := req.Context().Err(); err != nil {
				return nil, err
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if err := req.Context().Err(); err != nil {
				return nil, err
			}
			return rec.Result(), nil
		}),
	}
}

func TestPerformSelfUpdate_CheckAndJSON(t *testing.T) {
	const baseURL = "https://mock.github.test"

	t.Run("check with update available does not download or touch disk", func(t *testing.T) {
		var downloadHits atomic.Int32
		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/repos/karan-banwasi/Patchwork/releases/latest" {
				_ = json.NewEncoder(w).Encode(ghRelease{
					TagName: "v2.1.0",
					HTMLURL: "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.1.0",
				})
				return
			}
			downloadHits.Add(1)
			http.NotFound(w, r)
		}))

		// Even a managed winget path should not block --check
		wingetExe := `C:\Users\user\AppData\Local\Microsoft\WinGet\Packages\KaranBanwasi.Patchwork\pw.exe`
		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "windows", "amd64", wingetExe, selfUpdateOptions{
			check: true,
		}, &out)
		if err != nil {
			t.Fatalf("expected --check to succeed, got: %v", err)
		}
		if downloadHits.Load() != 0 {
			t.Errorf("expected 0 download hits during --check, got %d", downloadHits.Load())
		}
		got := out.String()
		if !strings.Contains(got, "v1.0.0 -> v2.1.0") {
			t.Errorf("expected update available message, got: %q", got)
		}
	})

	t.Run("check --json reports update_available without updated field", func(t *testing.T) {
		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ghRelease{
				TagName: "v2.5.0",
				HTMLURL: "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.5.0",
			})
		}))

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v2.0.0", "linux", "amd64", "/tmp/pw", selfUpdateOptions{
			check: true,
			json:  true,
		}, &out)
		if err != nil {
			t.Fatalf("performSelfUpdate failed: %v", err)
		}

		var raw map[string]any
		if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
			t.Fatalf("invalid JSON output %q: %v", out.String(), err)
		}
		if raw["schema_version"] != "1.0.0" {
			t.Errorf("expected schema_version 1.0.0, got %v", raw["schema_version"])
		}
		if raw["current_version"] != "v2.0.0" || raw["latest_version"] != "v2.5.0" {
			t.Errorf("unexpected versions in JSON: %+v", raw)
		}
		if raw["update_available"] != true || raw["channel"] != "stable" {
			t.Errorf("unexpected update_available/channel in JSON: %+v", raw)
		}
		if raw["release_url"] != "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.5.0" {
			t.Errorf("unexpected release_url: %v", raw["release_url"])
		}
		if _, hasUpdated := raw["updated"]; hasUpdated {
			t.Errorf("expected 'updated' field to be omitted in --check mode, got %v", raw["updated"])
		}
	})

	t.Run("full update --json reports updated true and emits pure JSON", func(t *testing.T) {
		payload := []byte("new-binary-json-test")
		sum := sha256.Sum256(payload)
		hashHex := hex.EncodeToString(sum[:])

		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/karan-banwasi/Patchwork/releases/latest":
				_ = json.NewEncoder(w).Encode(ghRelease{
					TagName: "v3.0.0",
					HTMLURL: "https://github.com/karan-banwasi/Patchwork/releases/tag/v3.0.0",
					Assets: []ghAsset{
						{Name: "pw-linux-amd64", BrowserDownloadURL: baseURL + "/dl/pw-linux-amd64"},
						{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/dl/SHA256SUMS"},
					},
				})
			case "/dl/SHA256SUMS":
				_, _ = fmt.Fprintf(w, "%s  pw-linux-amd64\n", hashHex)
			case "/dl/pw-linux-amd64":
				_, _ = w.Write(payload)
			default:
				http.NotFound(w, r)
			}
		}))

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw-linux-amd64")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v2.0.0", "linux", "amd64", exePath, selfUpdateOptions{
			skipAttestation: true,
			json:            true,
		}, &out)
		if err != nil {
			t.Fatalf("performSelfUpdate --json failed: %v", err)
		}

		var parsed selfUpdateJSONOutput
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("expected valid pure JSON output, got %q: %v", out.String(), err)
		}
		if !parsed.UpdateAvailable || parsed.Updated == nil || !*parsed.Updated {
			t.Errorf("expected update_available=true and updated=true, got %+v", parsed)
		}
		if parsed.LatestVersion != "v3.0.0" || parsed.Channel != "stable" {
			t.Errorf("unexpected parsed JSON fields: %+v", parsed)
		}
	})

	t.Run("full update --json when already up to date sets updated=false", func(t *testing.T) {
		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ghRelease{
				TagName: "v2.0.0",
				HTMLURL: "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.0.0",
			})
		}))

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw-linux-amd64")
		_ = os.WriteFile(exePath, []byte("current"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v2.0.0", "linux", "amd64", exePath, selfUpdateOptions{
			skipAttestation: true,
			json:            true,
		}, &out)
		if err != nil {
			t.Fatalf("performSelfUpdate failed: %v", err)
		}

		var parsed selfUpdateJSONOutput
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("expected valid JSON, got %q: %v", out.String(), err)
		}
		if parsed.UpdateAvailable || parsed.Updated == nil || *parsed.Updated {
			t.Errorf("expected update_available=false and updated=false, got %+v", parsed)
		}
	})
}

func TestPerformSelfUpdate_ChannelAndPrerelease(t *testing.T) {
	const baseURL = "https://mock.github.test"

	t.Run("prerelease channel picks newest non-draft prerelease and updates binary", func(t *testing.T) {
		payload := []byte("prerelease-binary-payload")
		sum := sha256.Sum256(payload)
		hashHex := hex.EncodeToString(sum[:])

		var requestedPath string
		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/karan-banwasi/Patchwork/releases":
				requestedPath = r.URL.RequestURI()
				releases := []ghRelease{
					{
						TagName:    "v3.0.0-alpha.1",
						Draft:      true,
						Prerelease: true,
						HTMLURL:    "https://github.com/karan-banwasi/Patchwork/releases/tag/v3.0.0-alpha.1",
					},
					{
						TagName:    "v2.1.0-beta.1",
						Prerelease: true,
						HTMLURL:    "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.1.0-beta.1",
					},
					{
						TagName:    "v2.1.0-rc.2",
						Prerelease: true,
						HTMLURL:    "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.1.0-rc.2",
						Assets: []ghAsset{
							{Name: "pw-linux-amd64", BrowserDownloadURL: baseURL + "/dl/pw-linux-amd64"},
							{Name: "SHA256SUMS", BrowserDownloadURL: baseURL + "/dl/SHA256SUMS"},
						},
					},
					{
						TagName:    "v2.0.0",
						Prerelease: false,
						HTMLURL:    "https://github.com/karan-banwasi/Patchwork/releases/tag/v2.0.0",
					},
				}
				_ = json.NewEncoder(w).Encode(releases)
			case "/dl/SHA256SUMS":
				_, _ = fmt.Fprintf(w, "%s  pw-linux-amd64\n", hashHex)
			case "/dl/pw-linux-amd64":
				_, _ = w.Write(payload)
			default:
				http.NotFound(w, r)
			}
		}))

		tmpDir := t.TempDir()
		exePath := filepath.Join(tmpDir, "pw-linux-amd64")
		_ = os.WriteFile(exePath, []byte("old"), 0755)

		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), client, baseURL, "v2.0.0", "linux", "amd64", exePath, selfUpdateOptions{
			prerelease:      true,
			skipAttestation: true,
			json:            true,
		}, &out)
		if err != nil {
			t.Fatalf("expected prerelease update to succeed, got: %v", err)
		}
		if requestedPath != "/repos/karan-banwasi/Patchwork/releases?per_page=10" {
			t.Errorf("expected releases?per_page=10 request, got %q", requestedPath)
		}

		var parsed selfUpdateJSONOutput
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON output: %v", err)
		}
		if parsed.LatestVersion != "v2.1.0-rc.2" || parsed.Channel != "prerelease" {
			t.Errorf("expected v2.1.0-rc.2 on prerelease channel, got %+v", parsed)
		}
		if parsed.Updated == nil || !*parsed.Updated {
			t.Errorf("expected updated=true, got %+v", parsed)
		}
		gotBytes, _ := os.ReadFile(exePath)
		if !bytes.Equal(gotBytes, payload) {
			t.Errorf("expected binary replaced with prerelease payload")
		}
	})

	t.Run("invalid channel returns error", func(t *testing.T) {
		var out bytes.Buffer
		err := performSelfUpdate(context.Background(), http.DefaultClient, "https://api.github.test", "v1.0.0", "linux", "amd64", "/tmp/pw", selfUpdateOptions{
			check:   true,
			channel: "nightly",
		}, &out)
		if err == nil || !strings.Contains(err.Error(), "invalid channel") {
			t.Fatalf("expected invalid channel error, got: %v", err)
		}
	})

	t.Run("CLI command honors defaults.channel from config and allows flag overrides", func(t *testing.T) {
		cfgPath := filepath.Join(t.TempDir(), "config.json")
		t.Setenv("PATCHWORK_CONFIG", cfgPath)
		cfg := &config.Config{}
		if err := cfg.Set("defaults.channel", "prerelease"); err != nil {
			t.Fatal(err)
		}
		if err := cfg.SaveTo(cfgPath); err != nil {
			t.Fatal(err)
		}

		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/karan-banwasi/Patchwork/releases":
				_ = json.NewEncoder(w).Encode([]ghRelease{
					{TagName: "v2.2.0-rc.1", Prerelease: true, HTMLURL: "https://example.test/rc1"},
				})
			case "/repos/karan-banwasi/Patchwork/releases/latest":
				_ = json.NewEncoder(w).Encode(ghRelease{
					TagName: "v2.1.0", HTMLURL: "https://example.test/v2.1.0",
				})
			default:
				http.NotFound(w, r)
			}
		}))

		origBase := selfUpdateAPIBaseURL
		origClient := selfUpdateHTTPClient
		origVer := version
		selfUpdateAPIBaseURL = baseURL
		selfUpdateHTTPClient = client
		version = "v2.0.0"
		t.Cleanup(func() {
			selfUpdateAPIBaseURL = origBase
			selfUpdateHTTPClient = origClient
			version = origVer
		})

		// 1. Default from config -> prerelease
		var out1 bytes.Buffer
		root1 := NewRootCmd()
		root1.SetOut(&out1)
		root1.SetArgs([]string{"self-update", "--check", "--json"})
		if err := root1.Execute(); err != nil {
			t.Fatalf("self-update --check --json failed: %v", err)
		}
		var res1 selfUpdateJSONOutput
		if err := json.Unmarshal(out1.Bytes(), &res1); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if res1.Channel != "prerelease" || res1.LatestVersion != "v2.2.0-rc.1" {
			t.Errorf("expected prerelease from config, got %+v", res1)
		}

		// 2. Explicit --channel stable overrides config
		var out2 bytes.Buffer
		root2 := NewRootCmd()
		root2.SetOut(&out2)
		root2.SetArgs([]string{"self-update", "--check", "--json", "--channel", "stable"})
		if err := root2.Execute(); err != nil {
			t.Fatalf("self-update --channel stable failed: %v", err)
		}
		var res2 selfUpdateJSONOutput
		if err := json.Unmarshal(out2.Bytes(), &res2); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if res2.Channel != "stable" || res2.LatestVersion != "v2.1.0" {
			t.Errorf("expected stable override, got %+v", res2)
		}

		// 3. --prerelease overrides --channel stable
		var out3 bytes.Buffer
		root3 := NewRootCmd()
		root3.SetOut(&out3)
		root3.SetArgs([]string{"self-update", "--check", "--json", "--channel", "stable", "--prerelease"})
		if err := root3.Execute(); err != nil {
			t.Fatalf("self-update --prerelease failed: %v", err)
		}
		var res3 selfUpdateJSONOutput
		if err := json.Unmarshal(out3.Bytes(), &res3); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if res3.Channel != "prerelease" || res3.LatestVersion != "v2.2.0-rc.1" {
			t.Errorf("expected --prerelease to force prerelease channel, got %+v", res3)
		}
	})
}

func TestSelfUpdateCachePath(t *testing.T) {
	customCache := filepath.Join(t.TempDir(), "custom-selfupdate.json")
	t.Setenv("PATCHWORK_SELFUPDATE_CACHE", customCache)
	if got := SelfUpdateCachePath(); got != customCache {
		t.Errorf("SelfUpdateCachePath() = %q, want %q", got, customCache)
	}

	t.Setenv("PATCHWORK_SELFUPDATE_CACHE", "")
	updatesCache := filepath.Join(t.TempDir(), "pw-cache", "updates-cache.json")
	t.Setenv("PATCHWORK_CACHE", updatesCache)
	wantDefault := filepath.Join(filepath.Dir(updatesCache), "selfupdate-cache.json")
	if got := SelfUpdateCachePath(); got != wantDefault {
		t.Errorf("SelfUpdateCachePath() default = %q, want %q", got, wantDefault)
	}
}

func TestPassiveSelfUpdateNotice(t *testing.T) {
	const baseURL = "https://mock.github.test"

	setupMockCheckRegistry := func(t *testing.T) {
		t.Helper()
		reg := pm.NewRegistry(&mockCheckManager{name: "mockpm"})
		registryOverride = reg
		t.Cleanup(func() { registryOverride = nil })
	}

	t.Run("fetches release, caches for 24h, and prints notice to stderr", func(t *testing.T) {
		setupMockCheckRegistry(t)
		cacheFile := filepath.Join(t.TempDir(), "selfupdate-cache.json")
		cfgFile := filepath.Join(t.TempDir(), "config.json")
		t.Setenv("PATCHWORK_SELFUPDATE_CACHE", cacheFile)
		t.Setenv("PATCHWORK_CONFIG", cfgFile)
		t.Setenv("PATCHWORK_OFFLINE", "")
		t.Setenv("PATCHWORK_NO_UPDATE_CHECK", "")

		var hits atomic.Int32
		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			_ = json.NewEncoder(w).Encode(ghRelease{
				TagName: "v1.9.0",
				HTMLURL: "https://github.com/karan-banwasi/Patchwork/releases/tag/v1.9.0",
			})
		}))

		origBase := selfUpdateAPIBaseURL
		origNoticeClient := selfUpdateNoticeClient
		origVer := version
		selfUpdateAPIBaseURL = baseURL
		selfUpdateNoticeClient = client
		version = "v1.0.0"
		t.Cleanup(func() {
			selfUpdateAPIBaseURL = origBase
			selfUpdateNoticeClient = origNoticeClient
			version = origVer
		})

		// First run: hits server, writes cache, prints notice to stderr
		var out1, err1 bytes.Buffer
		cmd1 := NewRootCmd()
		cmd1.SetOut(&out1)
		cmd1.SetErr(&err1)
		cmd1.SetArgs([]string{"check"})
		if err := cmd1.Execute(); err != nil {
			t.Fatalf("check failed: %v", err)
		}
		if hits.Load() != 1 {
			t.Errorf("expected 1 HTTP request on cold cache, got %d", hits.Load())
		}
		if !strings.Contains(err1.String(), "v1.0.0 -> v1.9.0") {
			t.Errorf("expected stderr notice on first run, got: %q", err1.String())
		}

		// Verify cache file written
		cached, ok := loadSelfUpdateCache(channelStable)
		if !ok || cached.LatestVersion != "v1.9.0" {
			t.Fatalf("expected valid cached entry v1.9.0, got ok=%v, cached=%+v", ok, cached)
		}

		// Second run within 24h: uses cache, 0 HTTP requests, still prints notice
		var out2, err2 bytes.Buffer
		cmd2 := NewRootCmd()
		cmd2.SetOut(&out2)
		cmd2.SetErr(&err2)
		cmd2.SetArgs([]string{"check"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("second check failed: %v", err)
		}
		if hits.Load() != 1 {
			t.Errorf("expected 0 additional HTTP requests within 24h, got total %d", hits.Load())
		}
		if !strings.Contains(err2.String(), "v1.0.0 -> v1.9.0") {
			t.Errorf("expected stderr notice from cache, got: %q", err2.String())
		}

		// Expired cache (>24h): refreshes via HTTP
		expired := SelfUpdateCache{
			CheckedAt:     time.Now().UTC().Add(-25 * time.Hour),
			LatestVersion: "v1.5.0",
			Channel:       channelStable,
		}
		if err := saveSelfUpdateCache(&expired); err != nil {
			t.Fatal(err)
		}
		var out3, err3 bytes.Buffer
		cmd3 := NewRootCmd()
		cmd3.SetOut(&out3)
		cmd3.SetErr(&err3)
		cmd3.SetArgs([]string{"check"})
		if err := cmd3.Execute(); err != nil {
			t.Fatalf("third check failed: %v", err)
		}
		if hits.Load() != 2 {
			t.Errorf("expected HTTP refresh after 24h expiry, got total %d", hits.Load())
		}
		if !strings.Contains(err3.String(), "v1.0.0 -> v1.9.0") {
			t.Errorf("expected refreshed notice, got: %q", err3.String())
		}
	})

	t.Run("does not print notice when already on latest version", func(t *testing.T) {
		setupMockCheckRegistry(t)
		cacheFile := filepath.Join(t.TempDir(), "selfupdate-cache.json")
		t.Setenv("PATCHWORK_SELFUPDATE_CACHE", cacheFile)
		t.Setenv("PATCHWORK_CONFIG", filepath.Join(t.TempDir(), "config.json"))
		t.Setenv("PATCHWORK_OFFLINE", "")
		t.Setenv("PATCHWORK_NO_UPDATE_CHECK", "")

		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ghRelease{TagName: "v1.9.0"})
		}))

		origBase := selfUpdateAPIBaseURL
		origNoticeClient := selfUpdateNoticeClient
		origVer := version
		selfUpdateAPIBaseURL = baseURL
		selfUpdateNoticeClient = client
		version = "v1.9.0"
		t.Cleanup(func() {
			selfUpdateAPIBaseURL = origBase
			selfUpdateNoticeClient = origNoticeClient
			version = origVer
		})

		var out, errBuf bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&errBuf)
		cmd.SetArgs([]string{"check"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("check failed: %v", err)
		}
		if errBuf.Len() != 0 {
			t.Errorf("expected empty stderr when up to date, got: %q", errBuf.String())
		}
	})

	t.Run("slow server does not stall CLI beyond grace wait", func(t *testing.T) {
		setupMockCheckRegistry(t)
		cacheFile := filepath.Join(t.TempDir(), "selfupdate-cache.json")
		t.Setenv("PATCHWORK_SELFUPDATE_CACHE", cacheFile)
		t.Setenv("PATCHWORK_CONFIG", filepath.Join(t.TempDir(), "config.json"))
		t.Setenv("PATCHWORK_OFFLINE", "")
		t.Setenv("PATCHWORK_NO_UPDATE_CHECK", "")

		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}))

		origBase := selfUpdateAPIBaseURL
		origNoticeClient := selfUpdateNoticeClient
		origVer := version
		selfUpdateAPIBaseURL = baseURL
		selfUpdateNoticeClient = client
		version = "v1.0.0"
		t.Cleanup(func() {
			selfUpdateAPIBaseURL = origBase
			selfUpdateNoticeClient = origNoticeClient
			version = origVer
		})

		start := time.Now()
		var out, errBuf bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&errBuf)
		cmd.SetArgs([]string{"check"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("check failed on slow update server: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed > 3*time.Second {
			t.Errorf("expected passive check not to stall CLI, took %s", elapsed)
		}
		if errBuf.Len() != 0 {
			t.Errorf("expected no notice on timeout, got %q", errBuf.String())
		}
	})

	t.Run("suppression conditions prevent network check and stderr notice", func(t *testing.T) {
		setupMockCheckRegistry(t)
		cacheFile := filepath.Join(t.TempDir(), "selfupdate-cache.json")
		t.Setenv("PATCHWORK_SELFUPDATE_CACHE", cacheFile)
		t.Setenv("PATCHWORK_CONFIG", filepath.Join(t.TempDir(), "config.json"))

		// Pre-populate cache with newer version so even cached notice MUST be suppressed
		if err := saveSelfUpdateCache(&SelfUpdateCache{
			CheckedAt:     time.Now().UTC(),
			LatestVersion: "v9.9.9",
			Channel:       channelStable,
		}); err != nil {
			t.Fatal(err)
		}

		var hits atomic.Int32
		client := newTestGitHubClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			_ = json.NewEncoder(w).Encode(ghRelease{TagName: "v9.9.9"})
		}))

		origBase := selfUpdateAPIBaseURL
		origClient := selfUpdateHTTPClient
		origNoticeClient := selfUpdateNoticeClient
		origVer := version
		selfUpdateAPIBaseURL = baseURL
		selfUpdateHTTPClient = client
		selfUpdateNoticeClient = client
		t.Cleanup(func() {
			selfUpdateAPIBaseURL = origBase
			selfUpdateHTTPClient = origClient
			selfUpdateNoticeClient = origNoticeClient
			version = origVer
		})

		cases := []struct {
			name string
			ver  string
			env  map[string]string
			args []string
		}{
			{
				name: "version is dev",
				ver:  "dev",
				args: []string{"check"},
			},
			{
				name: "--json flag",
				ver:  "v1.0.0",
				args: []string{"check", "--json"},
			},
			{
				name: "--quiet flag",
				ver:  "v1.0.0",
				args: []string{"check", "-q"},
			},
			{
				name: "--offline flag",
				ver:  "v1.0.0",
				args: []string{"check", "--offline"},
			},
			{
				name: "PATCHWORK_OFFLINE=1",
				ver:  "v1.0.0",
				env:  map[string]string{"PATCHWORK_OFFLINE": "1"},
				args: []string{"check"},
			},
			{
				name: "PATCHWORK_OFFLINE=true",
				ver:  "v1.0.0",
				env:  map[string]string{"PATCHWORK_OFFLINE": "true"},
				args: []string{"check"},
			},
			{
				name: "PATCHWORK_NO_UPDATE_CHECK=1",
				ver:  "v1.0.0",
				env:  map[string]string{"PATCHWORK_NO_UPDATE_CHECK": "1"},
				args: []string{"check"},
			},
			{
				name: "PATCHWORK_NO_UPDATE_CHECK=true",
				ver:  "v1.0.0",
				env:  map[string]string{"PATCHWORK_NO_UPDATE_CHECK": "true"},
				args: []string{"check"},
			},
			{
				name: "self-update subcommand",
				ver:  "v1.0.0",
				args: []string{"self-update", "--check"},
			},
			{
				name: "update alias subcommand",
				ver:  "v1.0.0",
				args: []string{"update", "--check"},
			},
			{
				name: "completion subcommand",
				ver:  "v1.0.0",
				args: []string{"completion", "powershell"},
			},
			{
				name: "version subcommand",
				ver:  "v1.0.0",
				args: []string{"version"},
			},
			{
				name: "help subcommand",
				ver:  "v1.0.0",
				args: []string{"help"},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("PATCHWORK_OFFLINE", "")
				t.Setenv("PATCHWORK_NO_UPDATE_CHECK", "")
				for k, v := range tc.env {
					t.Setenv(k, v)
				}
				version = tc.ver
				beforeHits := hits.Load()

				var out, errBuf bytes.Buffer
				cmd := NewRootCmd()
				cmd.SetOut(&out)
				cmd.SetErr(&errBuf)
				cmd.SetArgs(tc.args)
				if err := cmd.Execute(); err != nil {
					t.Fatalf("command %v failed: %v", tc.args, err)
				}

				if strings.Contains(errBuf.String(), "new version of Patchwork") {
					t.Errorf("expected passive notice to be suppressed for %s, got stderr: %q", tc.name, errBuf.String())
				}
				// Note: self-update / update subcommands intentionally hit the server for their own --check action,
				// while all other suppressed commands must make 0 passive check hits.
				if tc.args[0] != "self-update" && tc.args[0] != "update" {
					if hits.Load() != beforeHits {
						t.Errorf("expected 0 HTTP requests for %s, got %d", tc.name, hits.Load()-beforeHits)
					}
				}
			})
		}

		// Also verify cold cache makes 0 HTTP requests when suppressed
		_ = os.Remove(cacheFile)
		for _, tc := range cases {
			if tc.args[0] == "self-update" || tc.args[0] == "update" {
				continue
			}
			t.Run("cold_cache_"+tc.name, func(t *testing.T) {
				t.Setenv("PATCHWORK_OFFLINE", "")
				t.Setenv("PATCHWORK_NO_UPDATE_CHECK", "")
				for k, v := range tc.env {
					t.Setenv(k, v)
				}
				version = tc.ver
				beforeHits := hits.Load()

				var out, errBuf bytes.Buffer
				cmd := NewRootCmd()
				cmd.SetOut(&out)
				cmd.SetErr(&errBuf)
				cmd.SetArgs(tc.args)
				if err := cmd.Execute(); err != nil {
					t.Fatalf("command %v failed: %v", tc.args, err)
				}
				if hits.Load() != beforeHits {
					t.Errorf("expected 0 HTTP requests on cold cache for %s, got %d", tc.name, hits.Load()-beforeHits)
				}
			})
		}
	})
}
