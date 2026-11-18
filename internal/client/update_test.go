package client

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/Cmacle/TheHouseAlwaysWins/internal/release"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cmacle/TheHouseAlwaysWins/internal/proto"
)

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.6.0", "1.5.0", true}, {"1.10.0", "1.9.9", true}, {"2.0", "1.99.99", true},
		{"1.5.0", "1.5.0", false}, {"1.4.9", "1.5.0", false}, {"v1.6.0", "1.5.0", true},
		{"dev", "1.5.0", false}, {"1.6.0", "dev", false}, {"1.6.0-beta", "1.5.0", true}, {"", "1.0.0", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func releaseServer(t *testing.T, files map[string]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/v9.9.9/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(b))
	}))
	t.Cleanup(srv.Close)
	old, oldAPI := release.Base, release.API
	release.Base, release.API = srv.URL+"/v%s/", srv.URL+"/api/%s"
	t.Cleanup(func() { release.Base, release.API = old, oldAPI })
	return srv
}

func TestDownloadReleaseChecksums(t *testing.T) {
	body := strings.Repeat("new client ", 10000)
	sum := sha256.Sum256([]byte(body))
	good := hex.EncodeToString(sum[:])
	dir := t.TempDir()

	releaseServer(t, map[string]string{
		"SHA256SUMS.txt": good + "  app.bin\n" + strings.Repeat("0", 64) + "  other.bin\n",
		"app.bin":        body,
	})
	path, err := downloadRelease("9.9.9", "app.bin", dir, nil, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != body {
		t.Fatal("wrong content")
	}

	// tampered file: thrown away
	releaseServer(t, map[string]string{
		"SHA256SUMS.txt": good + "  app.bin\n",
		"app.bin":        body + "evil",
	})
	if _, err := downloadRelease("9.9.9", "app.bin", dir, nil, func(int64, int64) {}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("tampered download accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.bin.download")); err == nil {
		t.Fatal("bad download left behind")
	}

	// no checksum for this file: refused before downloading
	releaseServer(t, map[string]string{"SHA256SUMS.txt": good + "  something-else\n", "app.bin": body})
	if _, err := downloadRelease("9.9.9", "app.bin", dir, nil, func(int64, int64) {}); err == nil {
		t.Fatal("download without a checksum accepted")
	}

	// not released yet
	releaseServer(t, map[string]string{})
	if _, err := downloadRelease("9.9.9", "app.bin", dir, nil, func(int64, int64) {}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing release: %v", err)
	}
}

type updPlatform struct{ fakePlatform }

func (updPlatform) UpdateAsset() string        { return "app.bin" }
func (updPlatform) UpdateDir() string          { return os.TempDir() }
func (updPlatform) InstallUpdate(string) error { return nil }

func TestUpdateOffer(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "1.5.0"
	assets, _ := LoadAssets("")
	a := New(updPlatform{}, &Config{}, assets)
	offer := func(si proto.ServerInfo) bool {
		a.upd = updater{}
		a.serverInfo = si
		a.checkForUpdate()
		return a.updateOpen()
	}
	if !offer(proto.ServerInfo{ServerVersion: "1.6.0", Updates: true}) {
		t.Fatal("no offer from a newer server")
	}
	if offer(proto.ServerInfo{ServerVersion: "1.6.0", Updates: false}) {
		t.Fatal("offered when the server turned updates off")
	}
	if offer(proto.ServerInfo{ServerVersion: "1.5.0", Updates: true}) || offer(proto.ServerInfo{ServerVersion: "dev", Updates: true}) {
		t.Fatal("offered without a newer version")
	}
	// "later" sticks for that version
	a.upd = updater{dismissed: "1.6.0"}
	a.serverInfo = proto.ServerInfo{ServerVersion: "1.6.0", Updates: true}
	a.checkForUpdate()
	if a.updateOpen() {
		t.Fatal("offered again after 'later'")
	}
	// platforms that can't update never offer
	b := New(fakePlatform{}, &Config{}, assets)
	b.serverInfo = proto.ServerInfo{ServerVersion: "1.6.0", Updates: true}
	b.checkForUpdate()
	if b.updateOpen() {
		t.Fatal("offered on a platform without updates")
	}
}
