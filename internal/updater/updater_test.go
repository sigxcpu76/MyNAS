package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sigxcpu/nas/internal/version"
)

func TestFindMatchingAssets(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "nasctl-v1.6.0-linux-amd64", Size: 70000000, BrowserDownloadURL: "https://example.com/bin"},
		{Name: "nasctl-v1.6.0-linux-amd64.tar.gz", Size: 30000000, BrowserDownloadURL: "https://example.com/tar"},
		{Name: "nasctl-v1.6.0-linux-amd64.tar.gz.sha256", Size: 100, BrowserDownloadURL: "https://example.com/sha"},
		{Name: "nasctl-v1.6.0-linux-arm64", Size: 65000000, BrowserDownloadURL: "https://example.com/arm-bin"},
		{Name: "nasctl-v1.6.0-linux-arm64.tar.gz", Size: 28000000, BrowserDownloadURL: "https://example.com/arm-tar"},
	}

	raw, archive, sha := findMatchingAssets(assets, "linux", "amd64")
	if raw == nil || raw.Name != "nasctl-v1.6.0-linux-amd64" {
		t.Fatalf("expected raw binary match, got %+v", raw)
	}
	if archive == nil || archive.Name != "nasctl-v1.6.0-linux-amd64.tar.gz" {
		t.Fatalf("expected archive match, got %+v", archive)
	}
	if sha == nil || sha.Name != "nasctl-v1.6.0-linux-amd64.tar.gz.sha256" {
		t.Fatalf("expected sha match, got %+v", sha)
	}
}

func TestUpdaterCheck(t *testing.T) {
	version.Version = "v1.5.0"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/sigxcpu76/MyNAS/releases/latest" {
			rel := GitHubRelease{
				TagName:     "v1.6.0",
				Name:        "v1.6.0 Release",
				HTMLURL:     "https://github.com/sigxcpu76/MyNAS/releases/tag/v1.6.0",
				Body:        "### Changelog\n- Added automatic updates",
				PublishedAt: time.Now(),
				Assets: []GitHubAsset{
					{
						Name:               "nasctl-v1.6.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz",
						Size:               12345,
						BrowserDownloadURL: "https://example.com/asset.tar.gz",
					},
					{
						Name:               "nasctl-v1.6.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz.sha256",
						Size:               64,
						BrowserDownloadURL: "http://" + r.Host + "/sha256",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		if r.URL.Path == "/sha256" {
			_, _ = w.Write([]byte("abcdef1234567890  asset.tar.gz\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New("sigxcpu76/MyNAS")
	u.apiBaseURL = server.URL

	res, err := u.Check(context.Background(), true, "")
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}

	if !res.UpdateAvailable {
		t.Errorf("expected update to be available (current=%s, latest=%s)", res.CurrentVersion, res.LatestVersion)
	}
	if res.LatestVersion != "v1.6.0" {
		t.Errorf("expected latest version v1.6.0, got %s", res.LatestVersion)
	}
	if res.ExpectedSHA256 != "abcdef1234567890" {
		t.Errorf("expected sha 'abcdef1234567890', got %s", res.ExpectedSHA256)
	}
}

func TestDownloadAndExtractTarGz(t *testing.T) {
	// Create a mock tar.gz containing 'nasctl'
	var tarGzBuf []byte
	binaryContent := []byte("#!/bin/sh\necho 'hello nasctl'\n")

	{
		var gzBuf bytesBuffer
		gw := gzip.NewWriter(&gzBuf)
		tw := tar.NewWriter(gw)

		hdr := &tar.Header{
			Name: "nasctl",
			Mode: 0755,
			Size: int64(len(binaryContent)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(binaryContent); err != nil {
			t.Fatal(err)
		}
		tw.Close()
		gw.Close()
		tarGzBuf = gzBuf.Bytes()
	}

	h := sha256.New()
	h.Write(tarGzBuf)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(tarGzBuf)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "nasctl_extracted")

	u := New("sigxcpu76/MyNAS")
	hash, err := u.downloadAndExtract(context.Background(), server.URL+"/asset.tar.gz", destPath)
	if err != nil {
		t.Fatalf("downloadAndExtract failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("hash mismatch: got %s, want %s", hash, expectedHash)
	}

	extracted, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("could not read extracted binary: %v", err)
	}
	if string(extracted) != string(binaryContent) {
		t.Errorf("extracted content mismatch: got %q, want %q", string(extracted), string(binaryContent))
	}
}

type bytesBuffer struct {
	data []byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) Bytes() []byte {
	return b.data
}

func TestUpdaterSecurityHardening(t *testing.T) {
	ctx := context.Background()

	// 1. fetchReleaseByTag must reject path traversal in tag
	u := New("sigxcpu76/MyNAS")
	if _, err := u.fetchReleaseByTag(ctx, "../../users/evil"); err == nil {
		t.Fatalf("expected fetchReleaseByTag to reject path traversal tag")
	}

	// 2. Apply must refuse when release has no SHA-256 checksum asset
	version.Version = "v1.5.0"
	noShaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := GitHubRelease{
			TagName:     "v1.6.0",
			PublishedAt: time.Now(),
			Assets: []GitHubAsset{
				{
					Name:               "nasctl-v1.6.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz",
					Size:               128,
					BrowserDownloadURL: "http://" + r.Host + "/asset.tar.gz",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer noShaSrv.Close()

	u.apiBaseURL = noShaSrv.URL
	if _, err := u.Apply(ctx, "", false); err == nil {
		t.Fatalf("expected Apply to refuse update when SHA-256 checksum is missing")
	}

	// 3. downloadAndExtract must ignore README-nasctl.md and extract exact basename bin/nasctl
	var gzBuf bytesBuffer
	gw := gzip.NewWriter(&gzBuf)
	tw := tar.NewWriter(gw)
	writeEntry := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	writeEntry("README-nasctl.md", "documentation, not a binary")
	writeEntry("nasctl-v1.6.42-linux-amd64", "#!/bin/sh\necho real-binary\n")
	_ = tw.Close()
	_ = gw.Close()

	tarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzBuf.Bytes())
	}))
	defer tarSrv.Close()

	dest := filepath.Join(t.TempDir(), "nasctl-out")
	if _, err := u.downloadAndExtract(ctx, tarSrv.URL+"/release.tar.gz", dest); err != nil {
		t.Fatalf("downloadAndExtract failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(got) != "#!/bin/sh\necho real-binary\n" {
		t.Fatalf("extracted wrong archive member: %q", string(got))
	}
}
