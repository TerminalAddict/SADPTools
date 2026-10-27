package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"io"
	"net/url"
	"strings"
	"time"
)

type Firmware struct {
	Filename           string `json:"filename"`
	Bytes              int64  `json:"bytes"`
	SHA256             string `json:"sha256"`
	ExpectedSHA256     string `json:"expectedSHA256"`
	SourceURL          string `json:"sourceURL"`
	Vendor             string `json:"vendor"`
	Model              string `json:"model"`
	Hardware           string `json:"hardware"`
	Platform           string `json:"platform"`
	Region             string `json:"region"`
	Version            string `json:"version"`
	ReleaseNotes       string `json:"releaseNotes"`
	IntermediateBuilds string `json:"intermediateBuilds"`
	Provenance         string `json:"provenance"`
	Signature          string `json:"signature"`
	Imported           string `json:"imported"`
}

func (f Firmware) Validate() error {
	u, e := url.Parse(f.SourceURL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(f.SourceURL) > 2048 {
		return errors.New("attach an official HTTPS source URL without credentials, query or fragment; it will not be fetched")
	}
	if f.Vendor == "" || f.Model == "" || len(f.ReleaseNotes) > 8192 || len(f.IntermediateBuilds) > 4096 {
		return errors.New("supply vendor/model and bounded release notes")
	}
	if f.ExpectedSHA256 != "" {
		b, e := hex.DecodeString(f.ExpectedSHA256)
		if e != nil || len(b) != 32 {
			return errors.New("official checksum must be a SHA-256 hexadecimal value")
		}
	}
	for _, s := range []string{f.Vendor, f.Model, f.Hardware, f.Platform, f.Region, f.Version} {
		if len(s) > 256 || s != sanitize.Label(s) {
			return errors.New("invalid firmware metadata")
		}
	}
	return nil
}
func HashFirmware(ctx context.Context, r io.Reader, f Firmware) (Firmware, error) {
	if e := f.Validate(); e != nil {
		return f, e
	}
	f.Bytes = 0
	hash := sha256.New()
	buf := make([]byte, 64<<10)
	defer clear(buf)
	for {
		if ctx.Err() != nil {
			return f, ctx.Err()
		}
		n, e := r.Read(buf)
		f.Bytes += int64(n)
		if f.Bytes > 512<<20 {
			return f, errors.New("firmware exceeds 512 MiB")
		}
		hash.Write(buf[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			return f, errors.New("firmware could not be read")
		}
	}
	if f.Bytes == 0 {
		return f, errors.New("firmware file is empty")
	}
	f.SHA256 = hex.EncodeToString(hash.Sum(nil))
	f.Imported = time.Now().UTC().Format(time.RFC3339Nano)
	f.Signature = "unknown — no verified vendor signature adapter"
	f.Provenance = "Operator-supplied official source and compatibility metadata; source authenticity not independently verified"
	if f.ExpectedSHA256 != "" && !strings.EqualFold(f.ExpectedSHA256, f.SHA256) {
		return f, errors.New("firmware checksum does not match the supplied official checksum; do not use this file")
	}
	return f, nil
}

type FirmwareIdentity struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	Hardware string `json:"hardware"`
	Platform string `json:"platform"`
	Region   string `json:"region"`
	Version  string `json:"version"`
	Source   string `json:"source"`
}
type FirmwareMatch struct {
	Current FirmwareIdentity `json:"current"`
	State   string           `json:"state"`
	Checks  []Step           `json:"checks"`
	Upload  string           `json:"upload"`
}

func MatchFirmware(f Firmware, d FirmwareIdentity) FirmwareMatch {
	r := FirmwareMatch{Current: d, State: "identifiers-match-unverified", Checks: []Step{}, Upload: "disabled — firmware transfer/reboot requires a separately hardware-validated adapter"}
	fields := [][3]string{{"vendor", f.Vendor, d.Vendor}, {"model", f.Model, d.Model}, {"hardware", f.Hardware, d.Hardware}, {"platform", f.Platform, d.Platform}, {"region", f.Region, d.Region}}
	for _, field := range fields {
		status, detail := "observed", "Exact identifiers match"
		if field[1] == "" || field[2] == "" {
			status = "unknown"
			detail = "Unable to verify compatibility: missing identifier"
			if r.State != "blocked" {
				r.State = "unable-to-verify"
			}
		} else if field[1] != field[2] {
			status = "failed"
			detail = "Package and device identifiers differ"
			r.State = "blocked"
		}
		r.Checks = append(r.Checks, Step{Test: field[0], Status: status, Evidence: detail})
	}
	r.Checks = append(r.Checks, Step{Test: "Upgrade/downgrade path", Status: "unknown", Evidence: "No safe sequence inferred from version strings. Follow the supplied official release notes/intermediate-build instructions."}, Step{Test: "Provenance/signature", Status: "unknown", Evidence: f.Provenance + "; " + f.Signature})
	return r
}
