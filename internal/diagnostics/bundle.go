package diagnostics

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/mediaimage"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/version"
	"runtime"
	"sort"
	"strings"
	"time"
)

type BundleFile struct {
	Name        string `json:"name"`
	Bytes       int    `json:"bytes"`
	SHA256      string `json:"sha256"`
	Sensitivity string `json:"sensitivity"`
}
type BundleManifest struct {
	Format    string       `json:"format"`
	Version   string       `json:"version"`
	Platform  string       `json:"platform"`
	Created   string       `json:"created"`
	Files     []BundleFile `json:"files"`
	Redaction []string     `json:"redaction"`
}
type Bundle struct {
	Manifest BundleManifest    `json:"manifest"`
	Preview  map[string]string `json:"preview"`
	Digest   string            `json:"digest"`
	Images   map[string]string `json:"images"`
}

func PrepareBundle(files map[string][]byte) (Bundle, error) { return PrepareBundleImages(files, nil) }

func PrepareBundleImages(files, pictures map[string][]byte) (Bundle, error) {
	b := Bundle{Manifest: BundleManifest{Format: "sadptools-support-v1", Version: version.Value, Platform: runtime.GOOS + "/" + runtime.GOARCH, Created: time.Now().UTC().Format(time.RFC3339), Files: []BundleFile{}, Redaction: []string{"Credentials, authentication URLs, token fields and plate/image fields removed", "Raw packet captures excluded; image pixels included only when explicitly selected, never automatically anonymised", "Selected device/network identifiers remain unless anonymisation selected; review every preview file"}}, Preview: map[string]string{}, Images: map[string]string{}}
	if len(files)+len(pictures) > 30 || len(pictures) > 4 {
		return b, errors.New("choose at most 30 evidence items, including at most four approved screenshots")
	}
	names := []string{}
	for name := range files {
		if strings.ContainsAny(name, "/\\") || !strings.HasSuffix(name, ".json") {
			return b, errors.New("invalid bundle filename")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		data := files[name]
		total += len(data)
		if len(data) > 2<<20 || total > 12<<20 || !json.Valid(data) {
			return b, errors.New("bundle JSON exceeds limits or is invalid")
		}
		var value any
		_ = json.Unmarshal(data, &value)
		data = SafeJSON(value)
		sum := sha256.Sum256(data)
		b.Manifest.Files = append(b.Manifest.Files, BundleFile{name, len(data), hex.EncodeToString(sum[:]), "review required — metadata"})
		b.Preview[name] = string(data)
	}
	names = nil
	for name := range pictures {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.ContainsAny(name, "/\\") || !strings.HasSuffix(name, ".png") {
			return b, errors.New("invalid screenshot filename")
		}
		data, _, _, e := mediaimage.Normalise(pictures[name])
		if e != nil {
			return b, e
		}
		total += len(data)
		if len(data) > 4<<20 || total > 12<<20 {
			return b, errors.New("approved screenshots exceed bundle size limit")
		}
		sum := sha256.Sum256(data)
		b.Manifest.Files = append(b.Manifest.Files, BundleFile{name, len(data), hex.EncodeToString(sum[:]), "explicitly approved image pixels — faces/plates/text are NOT redacted"})
		b.Images[name] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
		clear(data)
	}
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	b.Digest = hex.EncodeToString(sum[:])
	return b, nil
}
func (b Bundle) Export(approvedDigest, passphrase string) ([]byte, error) {
	if b.Digest == "" || approvedDigest != b.Digest {
		return nil, errors.New("review the current bundle preview before export")
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	manifest, _ := json.MarshalIndent(b.Manifest, "", "  ")
	checks := strings.Builder{}
	put := func(name string, data []byte) error {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0600)
		f, e := z.CreateHeader(h)
		if e != nil {
			return e
		}
		_, e = f.Write(data)
		return e
	}
	for _, f := range b.Manifest.Files {
		data := []byte(b.Preview[f.Name])
		if v, ok := b.Images[f.Name]; ok {
			if !strings.HasPrefix(v, "data:image/png;base64,") {
				return nil, errors.New("invalid preview image")
			}
			var e error
			data, e = base64.StdEncoding.DecodeString(strings.TrimPrefix(v, "data:image/png;base64,"))
			if e != nil {
				return nil, e
			}
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			return nil, errors.New("bundle preview changed; prepare it again")
		}
		if e := put(f.Name, data); e != nil {
			return nil, e
		}
		fmt.Fprintf(&checks, "%s  %s\n", f.SHA256, f.Name)
	}
	sum := sha256.Sum256(manifest)
	fmt.Fprintf(&checks, "%x  manifest.json\n", sum)
	if e := put("manifest.json", manifest); e != nil {
		return nil, e
	}
	if e := put("SHA256SUMS", []byte(checks.String())); e != nil {
		return nil, e
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	if passphrase != "" {
		defer clear(out.Bytes())
		return vault.EncryptBackup(out.Bytes(), passphrase)
	}
	return out.Bytes(), nil
}
