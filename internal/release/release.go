// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

// Package release downloads official releases of the game from GitHub,
// checked against the release's SHA256SUMS.txt. The clients and the server
// update themselves with it.
package release

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Base is where release files are downloaded from; %s is the version.
var Base = "https://github.com/Cmacle/TheHouseAlwaysWins/releases/download/v%s/"

// API is the fallback when the direct links don't work: the GitHub API's
// description of the release, which links every file.
var API = "https://api.github.com/repos/Cmacle/TheHouseAlwaysWins/releases/tags/v%s"

// LatestAPI describes the newest release.
var LatestAPI = "https://api.github.com/repos/Cmacle/TheHouseAlwaysWins/releases/latest"

// Latest asks GitHub for the newest released version ("1.7.0").
func Latest(agent string) (string, error) {
	url, accept := LatestAPI, "application/vnd.github+json"
	test := os.Getenv("HOUSE_TEST_RELEASES") // for testing updates locally
	if test != "" {
		url, accept = strings.TrimRight(test, "/")+"/latest.json", ""
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", agent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't reach GitHub: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub said HTTP %d", resp.StatusCode)
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return "", err
	}
	v := strings.TrimPrefix(rel.Tag, "v")
	if _, ok := Parse(v); !ok {
		return "", fmt.Errorf("odd release tag %q", rel.Tag)
	}
	return v, nil
}

// Newer reports whether version a is newer than b ("1.10.0" > "1.9.2").
// Anything that isn't a version number (like "dev") is never newer or older.
func Newer(a, b string) bool {
	va, ok1 := Parse(a)
	vb, ok2 := Parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// Parse reads "1.2.3" (a leading v, missing parts and a "-beta" style
// suffix are fine).
func Parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Download fetches one release file and checks it against the release's
// SHA256SUMS.txt. It returns the path of the verified file.
func Download(version, asset, dir, agent string, cancel <-chan struct{}, progress func(got, size int64)) (string, error) {
	base := fmt.Sprintf(Base, version)
	if t := os.Getenv("HOUSE_TEST_RELEASES"); t != "" { // for testing updates locally
		base = strings.TrimRight(t, "/") + "/"
	}
	hc := &http.Client{Timeout: 15 * time.Minute}
	fetch := func(url, accept string) (*http.Response, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", agent)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("couldn't reach GitHub: %v", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("the v%s download isn't available yet (HTTP %d)", version, resp.StatusCode)
		}
		return resp, nil
	}
	var apiAssets map[string]string // file name -> API download link
	get := func(url string) (*http.Response, error) {
		resp, err := fetch(url, "")
		if err == nil || os.Getenv("HOUSE_TEST_RELEASES") != "" {
			return resp, err
		}
		// the direct link didn't work: ask the API where the file is
		if apiAssets == nil {
			apiAssets = map[string]string{}
			if r, aerr := fetch(fmt.Sprintf(API, version), "application/vnd.github+json"); aerr == nil {
				var rel struct {
					Assets []struct {
						Name string `json:"name"`
						URL  string `json:"url"`
					} `json:"assets"`
				}
				json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&rel)
				r.Body.Close()
				for _, a := range rel.Assets {
					apiAssets[a.Name] = a.URL
				}
			}
		}
		name := url[strings.LastIndex(url, "/")+1:]
		if link, ok := apiAssets[name]; ok && strings.HasPrefix(link, "https://api.github.com/") {
			if resp, aerr := fetch(link, "application/octet-stream"); aerr == nil {
				return resp, nil
			}
		}
		return nil, err
	}
	// the checksum list first
	resp, err := get(base + "SHA256SUMS.txt")
	if err != nil {
		return "", err
	}
	want := ""
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
		}
	}
	resp.Body.Close()
	if len(want) != 64 {
		return "", fmt.Errorf("v%s has no checksum for %s", version, asset)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, asset+".download")
	resp, err = get(base + asset)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var got int64
	buf := make([]byte, 64*1024)
	const limit = 400 << 20
	for {
		select {
		case <-cancel:
			f.Close()
			os.Remove(path)
			return "", errors.New("cancelled")
		default:
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				os.Remove(path)
				return "", err
			}
			h.Write(buf[:n])
			got += int64(n)
			progress(got, resp.ContentLength)
			if got > limit {
				f.Close()
				os.Remove(path)
				return "", errors.New("download is too big")
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(path)
			return "", fmt.Errorf("download failed: %v", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		os.Remove(path)
		return "", errors.New("the download didn't match its checksum, so it was thrown away")
	}
	return path, nil
}
