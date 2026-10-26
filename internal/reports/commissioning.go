package reports

import (
	"bytes"
	"codeberg.org/go-pdf/fpdf"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/mediaimage"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"html/template"
	"sort"
	"strings"
	"time"
)

type Section struct {
	Title   string     `json:"title"`
	Note    string     `json:"note"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}
type Figure struct {
	ImageID string                `json:"imageID"`
	Caption string                `json:"caption"`
	Markers []installation.Marker `json:"markers"`
}
type Snapshot struct {
	Version         int       `json:"version"`
	At              string    `json:"at"`
	Title           string    `json:"title"`
	Site            string    `json:"site"`
	Customer        string    `json:"customer"`
	Address         string    `json:"address"`
	Technician      string    `json:"technician"`
	Date            string    `json:"date"`
	Project         string    `json:"project"`
	Company         string    `json:"company"`
	BusinessDetails string    `json:"businessDetails"`
	Summary         string    `json:"summary"`
	Signoff         string    `json:"signoff"`
	Redacted        bool      `json:"redacted"`
	Sections        []Section `json:"sections"`
	Figures         []Figure  `json:"figures"`
}
type ImageLoader func(string) ([]byte, error)

// CleanJSON is for non-secret evidence only. Whitelisted report construction is
// still required: credential records/opaque configuration are never passed in.
func CleanJSON(raw []byte, redact bool) (string, error) {
	var v any
	if len(raw) > 4<<20 || json.Unmarshal(raw, &v) != nil {
		return "", errors.New("invalid evidence")
	}
	var clean func(any, string) any
	clean = func(v any, key string) any {
		switch x := v.(type) {
		case map[string]any:
			m := map[string]any{}
			for k, w := range x {
				low := strings.ToLower(k)
				if sanitize.SensitiveKey(k) || low == "xaddrs" || low == "url" || low == "raw" {
					continue
				}
				if redact && (strings.Contains(low, "address") || strings.Contains(low, "ipv4") || low == "ip" || strings.Contains(low, "serial") || low == "mac" || low == "host" || low == "gateway" || low == "subnetmask") {
					m[k] = "[redacted]"
				} else {
					m[k] = clean(w, k)
				}
			}
			return m
		case []any:
			for i := range x {
				x[i] = clean(x[i], key)
			}
			return x
		case string:
			return sanitize.Text(x)
		default:
			return v
		}
	}
	b, e := json.MarshalIndent(clean(v, ""), "", "  ")
	return string(b), e
}
func (s Snapshot) Validate() error {
	if s.Version != 1 || len(s.Title) > 300 || len(s.Sections) > 128 || len(s.Figures) > 64 {
		return errors.New("report snapshot exceeds supported limits")
	}
	if _, e := time.Parse(time.RFC3339Nano, s.At); e != nil {
		return errors.New("report timestamp invalid")
	}
	for _, x := range s.Sections {
		if len(x.Headers) > 16 || len(x.Rows) > 10000 {
			return errors.New("report table exceeds limits")
		}
		for _, r := range x.Rows {
			if len(r) != len(x.Headers) {
				return errors.New("report row has wrong column count")
			}
		}
	}
	return nil
}
func Commissioning(format string, s Snapshot, load ImageLoader) ([]byte, error) {
	if e := s.Validate(); e != nil {
		return nil, e
	}
	switch format {
	case "json":
		return json.MarshalIndent(s, "", "  ")
	case "csv":
		rows := [][]string{}
		for _, sec := range s.Sections {
			for _, row := range sec.Rows {
				rows = append(rows, []string{sec.Title, strings.Join(sec.Headers, " | "), strings.Join(row, " | ")})
			}
		}
		return CSV([]string{"Section", "Fields", "Values"}, rows)
	case "html":
		return commissioningHTML(s, load)
	case "pdf":
		return commissioningPDF(s, load)
	default:
		return nil, errors.New("choose PDF, HTML, CSV or JSON")
	}
}

type htmlFigure struct {
	Caption string
	Source  template.URL
	Markers []installation.Marker
}

func commissioningHTML(s Snapshot, load ImageLoader) ([]byte, error) {
	figs := []htmlFigure{}
	for _, f := range s.Figures {
		data, e := load(f.ImageID)
		if e != nil {
			return nil, e
		}
		data, _, _, e = mediaimage.Normalise(data)
		if e != nil {
			return nil, e
		}
		figs = append(figs, htmlFigure{sanitize.Text(f.Caption), template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(data)), f.Markers})
	}
	const page = `<!doctype html><html lang="en"><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'"><title>{{.S.Title}}</title><style>body{font:14px system-ui,sans-serif;color:#173044;margin:36px;line-height:1.45}h1{font-size:30px;border-bottom:4px solid #13877e}h2{margin-top:30px;font-size:20px}small{color:#526579}table{width:100%;border-collapse:collapse;table-layout:fixed}td,th{border:1px solid #ccd5dc;padding:7px;text-align:left;overflow-wrap:anywhere;white-space:pre-wrap;vertical-align:top}th{background:#e9f2f3}p{white-space:pre-wrap;overflow-wrap:anywhere}.figure{position:relative;max-width:100%;margin:auto}.figure img{width:100%;display:block}.marker{position:absolute;background:#007b70;color:white;border:2px solid white;border-radius:8px;padding:3px;transform:translate(-50%,-50%);font-size:11px}figure{margin:24px 0}footer{border-top:1px solid #ccc;margin-top:30px} @media print{body{margin:0}tr,figure{break-inside:avoid}thead{display:table-header-group}h2{break-after:avoid}}</style><h1>{{.S.Title}}</h1><p><b>{{.S.Company}}</b><br>{{.S.BusinessDetails}}</p><h2>{{.S.Site}}</h2><p>Customer: {{.S.Customer}}<br>Site: {{.S.Address}}<br>Technician: {{.S.Technician}} · Date: {{.S.Date}}<br>Project: {{.S.Project}}</p><small>Snapshot {{.S.At}} · Schema {{.S.Version}}{{if .S.Redacted}} · Client-facing redaction enabled{{end}}</small><h2>Executive summary</h2><p>{{.S.Summary}}</p>{{range .S.Sections}}<h2>{{.Title}}</h2><p>{{.Note}}</p>{{if .Headers}}<table><thead><tr>{{range .Headers}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody></table>{{end}}{{end}}{{range .Figures}}<figure><div class="figure"><img src="{{.Source}}" alt="Site evidence">{{range .Markers}}<span class="marker" style="left:{{percent .X}}%;top:{{percent .Y}}%">{{.Label}}</span>{{end}}</div><figcaption>{{.Caption}}</figcaption></figure>{{end}}<h2>Acceptance / signoff</h2><p>{{.S.Signoff}}</p><footer>SADPTools · Saved evidence and technician observations. Not tested is not a pass. Coverage markings are approximate manual annotations. No credentials are included.</footer></html>`
	t, e := template.New("commissioning").Funcs(template.FuncMap{"percent": func(v float64) string { return fmt.Sprintf("%.4f", v*100) }}).Parse(page)
	if e != nil {
		return nil, e
	}
	var b bytes.Buffer
	e = t.Execute(&b, struct {
		S       Snapshot
		Figures []htmlFigure
	}{s, figs})
	return b.Bytes(), e
}
func commissioningPDF(s Snapshot, load ImageLoader) ([]byte, error) {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetMargins(15, 15, 15)
	p.SetAutoPageBreak(true, 18)
	p.SetCatalogSort(true)
	stamp, _ := time.Parse(time.RFC3339Nano, s.At)
	p.SetCreationDate(stamp)
	p.SetModificationDate(stamp)
	p.SetTitle(s.Title, true)
	p.SetAuthor(s.Company, true)
	p.SetCreator("SADPTools", true)
	p.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	p.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	p.SetFont("Go", "", 10)
	p.SetFooterFunc(func() {
		p.SetY(-13)
		p.SetFont("Go", "", 8)
		p.SetTextColor(80, 90, 100)
		p.CellFormat(180, 5, fmt.Sprintf("SADPTools · %s · Page %d", s.Date, p.PageNo()), "", 0, "R", false, 0, "")
	})
	p.AddPage()
	heading := func(text string) {
		if p.GetY() > 255 {
			p.AddPage()
		}
		p.SetTextColor(15, 105, 98)
		p.SetFont("Go", "B", 16)
		p.MultiCell(180, 8, sanitize.Text(text), "", "L", false)
		p.SetTextColor(25, 40, 55)
		p.SetFont("Go", "", 10)
	}
	para := func(text string) { p.MultiCell(180, 5, sanitize.Text(text), "", "L", false); p.Ln(2) }
	heading(s.Title)
	para(s.Company + "\n" + s.BusinessDetails)
	heading(s.Site)
	para("Customer: " + s.Customer + "\nSite: " + s.Address + "\nTechnician: " + s.Technician + " · " + s.Date + "\nProject: " + s.Project + "\nSnapshot: " + s.At)
	heading("Executive summary")
	para(s.Summary)
	for _, sec := range s.Sections {
		heading(sec.Title)
		para(sec.Note)
		if len(sec.Headers) == 0 {
			continue
		}
		w := 180 / float64(len(sec.Headers))
		row := func(values []string, bold bool) {
			p.SetFont("Go", "", 9)
			if bold {
				p.SetFont("Go", "B", 9)
			}
			maxLines := 1
			lines := make([][]string, len(values))
			for i, v := range values {
				lines[i] = p.SplitText(sanitize.Text(v), w-3)
				if len(lines[i]) > maxLines {
					maxLines = len(lines[i])
				}
			}
			// Split exceptionally long cells over multiple rows/pages without clipping.
			for start := 0; start < maxLines; {
				avail := int((277-p.GetY())/4.5) - 1
				if avail < 2 {
					p.AddPage()
					avail = 50
				}
				n := maxLines - start
				if n > avail {
					n = avail
				}
				h := float64(n)*4.5 + 2
				x, y := p.GetX(), p.GetY()
				for i := range values {
					end := start + n
					if end > len(lines[i]) {
						end = len(lines[i])
					}
					value := ""
					if start < len(lines[i]) {
						value = strings.Join(lines[i][start:end], "\n")
					}
					p.Rect(x+float64(i)*w, y, w, h, "D")
					p.SetXY(x+float64(i)*w+1, y+1)
					p.MultiCell(w-2, 4.5, value, "", "L", false)
				}
				p.SetXY(x, y+h)
				start += n
			}
		}
		row(sec.Headers, true)
		for _, r := range sec.Rows {
			row(r, false)
		}
		p.Ln(4)
	}
	for i, f := range s.Figures {
		data, e := load(f.ImageID)
		if e != nil {
			return nil, e
		}
		data, w, h, e := mediaimage.Normalise(data)
		if e != nil {
			return nil, e
		}
		p.AddPage()
		heading(f.Caption)
		width := 180.0
		height := width * float64(h) / float64(w)
		if height > 205 {
			height = 205
			width = height * float64(w) / float64(h)
		}
		x, y := 15+(180-width)/2, p.GetY()
		name := fmt.Sprintf("evidence-%d", i)
		p.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(data))
		p.ImageOptions(name, x, y, width, height, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
		p.SetFont("Go", "B", 8)
		for _, m := range f.Markers {
			mx, my := x+m.X*width, y+m.Y*height
			p.SetFillColor(10, 145, 130)
			p.Circle(mx, my, 2, "F")
			p.SetXY(mx-15, my+2)
			p.SetTextColor(10, 55, 60)
			p.MultiCell(30, 4, sanitize.Label(m.Label), "", "C", false)
		}
		p.SetXY(15, y+height+4)
		p.SetFont("Go", "", 9)
		para("Image evidence. Plan coordinates and coverage are approximate manual annotations.")
	}
	heading("Acceptance / signoff")
	para(s.Signoff)
	para("Recorded evidence and technician observations. Not tested is not a pass. No credentials included.")
	var out bytes.Buffer
	e := p.Output(&out)
	return out.Bytes(), e
}

// Deterministic section ordering is useful when evidence was gathered concurrently.
func SortSections(s []Section) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].Title < s[j].Title })
}

// RedactSnapshot also removes known device/customer values from free-form labels
// and notes, while preserving internal asset/device IDs used by the renderer.
func RedactSnapshot(s Snapshot, values []string) Snapshot {
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	raw, _ := json.Marshal(s)
	var v any
	_ = json.Unmarshal(raw, &v)
	var visit func(any, string) any
	visit = func(v any, key string) any {
		switch x := v.(type) {
		case map[string]any:
			for k, w := range x {
				x[k] = visit(w, k)
			}
			return x
		case []any:
			for i := range x {
				x[i] = visit(x[i], key)
			}
			return x
		case string:
			if key == "imageID" || key == "deviceID" || key == "id" {
				return x
			}
			for _, s := range values {
				if len(s) >= 4 {
					x = strings.ReplaceAll(x, s, "[redacted]")
				}
			}
			return x
		default:
			return v
		}
	}
	raw, _ = json.Marshal(visit(v, ""))
	_ = json.Unmarshal(raw, &s)
	return s
}
