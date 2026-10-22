package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const repository = "TerminalAddict/SADPTools"
const ReleasesURL = "https://github.com/" + repository + "/releases"
const maxInstallerSize int64 = 512 << 20

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	State  string `json:"state"`
	Digest string `json:"digest"`
}
type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

// Offer is retained in Go; the frontend cannot supply download URLs or paths.
type Offer struct {
	Version, Name, URL, ReleaseURL, SHA256 string
	Size                                   int64
}

type Client struct{ http *http.Client }

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Minute, CheckRedirect: checkRedirect}}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 || !trustedURL(req.URL) {
		return errors.New("refusing an update redirect outside GitHub HTTPS hosts")
	}
	return nil
}

func trustedURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}

func (c *Client) get(ctx context.Context, address string) (*http.Response, error) {
	u, err := url.Parse(address)
	if err != nil || !trustedURL(u) {
		return nil, errors.New("invalid update URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SADPTools-update-checker")
	if u.Hostname() == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			return nil, errors.New("GitHub refused the request or its rate limit was reached; try again later")
		}
		return nil, fmt.Errorf("GitHub returned HTTP %d; try again later", resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) small(ctx context.Context, address string, limit int64) ([]byte, error) {
	r, err := c.get(ctx, address)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("GitHub update metadata exceeded its size limit")
	}
	return b, err
}

func (c *Client) Check(ctx context.Context, current, channel string) (*Offer, error) {
	installed, err := parseVersion(current)
	if err != nil {
		return nil, err
	}
	if channel != "stable" && channel != "beta" {
		return nil, errors.New("update channel must be stable or beta")
	}
	if installed.channel == "dev" {
		return nil, errors.New("development builds do not install releases; use a versioned release build")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var latest *release
	best := installed
	for page := 1; ; page++ {
		if page > 10 {
			return nil, errors.New("too many GitHub release pages; check releases manually")
		}
		body, err := c.small(ctx, fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", repository, page), 4<<20)
		if err != nil {
			return nil, err
		}
		var releases []release
		if err := json.Unmarshal(body, &releases); err != nil {
			return nil, errors.New("GitHub returned invalid release metadata")
		}
		for _, r := range releases {
			v, err := parseVersion(r.Tag)
			if err != nil || !strings.HasPrefix(r.Tag, "v") || r.Draft || v.channel == "dev" ||
				r.Prerelease != (v.channel != "") || (channel == "stable" && r.Prerelease) {
				continue
			}
			if v.newer(best) {
				best, latest = v, &r
			}
		}
		if len(releases) < 100 {
			break
		}
	}
	if latest == nil {
		return nil, nil
	}
	value := strings.TrimPrefix(latest.Tag, "v")
	name := "SADPTools-" + value + "-Setup-windows-amd64.exe"
	find := func(name string, limit int64) (asset, error) {
		var found asset
		for _, a := range latest.Assets {
			if a.Name != name {
				continue
			}
			if found.Name != "" || a.State != "uploaded" || a.Size < 1 || a.Size > limit ||
				a.URL != ReleasesURL+"/download/"+latest.Tag+"/"+name {
				return asset{}, errors.New("release contains invalid or duplicate update assets")
			}
			found = a
		}
		if found.Name == "" {
			return asset{}, fmt.Errorf("release %s is missing %s", latest.Tag, name)
		}
		return found, nil
	}
	installer, err := find(name, maxInstallerSize)
	if err != nil {
		return nil, err
	}
	sums, err := find("SHA256SUMS", 128<<10)
	if err != nil {
		return nil, err
	}
	data, err := c.small(ctx, sums.URL, 128<<10)
	if err != nil {
		return nil, err
	}
	hash, err := checksum(data, name)
	if err != nil {
		return nil, err
	}
	if installer.Digest != "" && installer.Digest != "sha256:"+hash {
		return nil, errors.New("release checksum disagrees with GitHub's asset digest")
	}
	return &Offer{Version: value, Name: name, URL: installer.URL, ReleaseURL: ReleasesURL + "/tag/" + latest.Tag, SHA256: hash, Size: installer.Size}, nil
}

func checksum(data []byte, name string) (string, error) {
	var result string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		b, err := hex.DecodeString(fields[0])
		if result != "" || err != nil || len(b) != sha256.Size {
			return "", errors.New("invalid or duplicate installer checksum")
		}
		result = strings.ToLower(fields[0])
	}
	if result == "" {
		return "", errors.New("release has no installer checksum")
	}
	return result, nil
}

func VerifyFile(path string, offer Offer) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != offer.Size {
		return errors.New("downloaded installer size or file type is invalid")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != offer.SHA256 {
		return errors.New("installer checksum verification failed; download it again")
	}
	var magic [2]byte
	if _, err = f.ReadAt(magic[:], 0); err != nil || string(magic[:]) != "MZ" {
		return errors.New("download is not a Windows executable")
	}
	return nil
}

type progressWriter struct {
	n      int64
	report func(int64)
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.n += int64(len(b))
	if w.report != nil {
		w.report(w.n)
	}
	return len(b), nil
}

func (c *Client) Download(ctx context.Context, offer Offer, cache string, progress func(int64)) (path string, err error) {
	decoded, hashErr := hex.DecodeString(offer.SHA256)
	if offer.Size < 2 || offer.Size > maxInstallerSize || filepath.Base(offer.Name) != offer.Name ||
		offer.Name == "." || hashErr != nil || len(decoded) != sha256.Size {
		return "", errors.New("invalid installer download metadata")
	}
	if err = os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(cache, "update-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	f, err := os.OpenFile(filepath.Join(dir, "installer.part"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r, err := c.get(ctx, offer.URL)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	n, err := io.Copy(io.MultiWriter(f, &progressWriter{report: progress}), io.LimitReader(r.Body, offer.Size+1))
	if err != nil {
		return "", err
	}
	if n != offer.Size {
		return "", errors.New("installer download was incomplete or exceeded its expected size")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = VerifyFile(f.Name(), offer); err != nil {
		return "", err
	}
	path = filepath.Join(dir, offer.Name)
	err = os.Rename(f.Name(), path)
	return path, err
}
