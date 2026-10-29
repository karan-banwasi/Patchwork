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
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
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
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
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
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "amd64", exePath, selfUpdateOptions{skipAttestation: true}, &out)
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
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "amd64", exePath, selfUpdateOptions{skipAttestation: false}, &out)
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
		err := performSelfUpdate(context.Background(), client, baseURL, "v1.0.0", "amd64", exePath, selfUpdateOptions{skipAttestation: false}, &out)
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
