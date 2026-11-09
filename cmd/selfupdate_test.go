package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
					attResp := ghAttestationsResponse{
						Attestations: make([]struct {
							Bundle json.RawMessage `json:"bundle"`
						}, 1),
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
