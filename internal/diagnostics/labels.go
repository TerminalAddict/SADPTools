package diagnostics

import (
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/skip2/go-qrcode"
	"html"
	"regexp"
	"strings"
)

var opaqueID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Label struct {
	SiteID   string   `json:"siteID"`
	AssetID  string   `json:"assetID"`
	Lines    []string `json:"lines"`
	WidthMM  int      `json:"widthMM"`
	HeightMM int      `json:"heightMM"`
	Copies   int      `json:"copies"`
}

func AssetCode(site, id string) (string, error) {
	if !opaqueID.MatchString(site) || !opaqueID.MatchString(id) {
		return "", errors.New("invalid local asset identity")
	}
	return "SADPTOOLS:ASSET:1:" + site + ":" + id, nil
}
func ParseAssetCode(code string) (site, id string, err error) {
	parts := strings.Split(strings.TrimSpace(code), ":")
	if len(parts) != 5 || strings.Join(parts[:3], ":") != "SADPTOOLS:ASSET:1" {
		return "", "", errors.New("not a SADPTools asset code")
	}
	if _, err = AssetCode(parts[3], parts[4]); err != nil {
		return "", "", err
	}
	return parts[3], parts[4], nil
}

// LabelSVG uses a fixed opaque payload. Free text never enters the QR code.
func LabelSVG(l Label) ([]byte, error) {
	code, e := AssetCode(l.SiteID, l.AssetID)
	if e != nil {
		return nil, e
	}
	if l.WidthMM < 50 || l.WidthMM > 100 || l.HeightMM < 25 || l.HeightMM > 60 || l.Copies < 1 || l.Copies > 24 || len(l.Lines) > 6 {
		return nil, errors.New("labels require 50–100 × 25–60 mm, 1–24 copies and at most six text lines")
	}
	for _, v := range l.Lines {
		if len([]rune(v)) > 38 || sanitize.Label(v) != strings.TrimSpace(v) {
			return nil, errors.New("label lines must be short plain text without credentials or URLs")
		}
	}
	qr, e := qrcode.New(code, qrcode.Medium)
	if e != nil {
		return nil, e
	}
	matrix := qr.Bitmap()
	// Up to two columns on A4; exact millimetre geometry with printable margins.
	cols := 1
	if l.Copies > 1 {
		cols = 2
	}
	rows := (l.Copies + cols - 1) / cols
	w, h := l.WidthMM*cols, l.HeightMM*rows
	if h > 277 {
		return nil, errors.New("too many labels for one A4 sheet; reduce copies")
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%dmm" height="%dmm" viewBox="0 0 %d %d"><rect width="100%%" height="100%%" fill="white"/>`, w, h, w, h)
	for i := 0; i < l.Copies; i++ {
		x, y := (i%cols)*l.WidthMM, (i/cols)*l.HeightMM
		fmt.Fprintf(&b, `<g transform="translate(%d %d)"><rect x="0.2" y="0.2" width="%.1f" height="%.1f" rx="1" fill="none" stroke="#888" stroke-width=".2"/>`, x, y, float64(l.WidthMM)-.4, float64(l.HeightMM)-.4)
		size := float64(l.HeightMM - 4)
		if size > 30 {
			size = 30
		}
		scale := size / float64(len(matrix))
		fmt.Fprintf(&b, `<g transform="translate(1 2) scale(%f)" fill="black" shape-rendering="crispEdges">`, scale)
		for y, row := range matrix {
			for x, dark := range row {
				if dark {
					fmt.Fprintf(&b, `<rect x="%d" y="%d" width="1" height="1"/>`, x, y)
				}
			}
		}
		b.WriteString("</g>")
		textX := size + 2
		available := float64(l.WidthMM) - textX - 2
		for j, line := range l.Lines {
			font := 2.6
			need := float64(len([]rune(line))) * font * .6
			if need > available {
				font = available / (float64(len([]rune(line))) * .6)
			}
			if font < 1.4 {
				return nil, errors.New("label text is too long for this size; shorten it or choose a wider label")
			}
			fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" font-family="sans-serif" font-size="%.2f" fill="black">%s</text>`, textX, 6+float64(j)*min(4, float64(l.HeightMM-12)/float64(max(1, len(l.Lines)-1))), font, html.EscapeString(line))
		}
		fmt.Fprintf(&b, `<text x="%.2f" y="%d" font-family="sans-serif" font-size="1.6">Asset %s</text></g>`, textX, l.HeightMM-3, l.AssetID[:8])
	}
	b.WriteString("</svg>")
	return []byte(b.String()), nil
}
