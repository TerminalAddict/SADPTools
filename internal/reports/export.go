// Package reports generates offline, secret-free technician/customer reports.
package reports

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"html/template"
	"strings"
)

func Cell(s string) string {
	trim := strings.TrimLeft(s, " \t\r\n")
	if trim != "" && strings.ContainsRune("=+-@", rune(trim[0])) {
		return "'" + s
	}
	return s
}
func CSV(headers []string, rows [][]string) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if e := w.Write(headers); e != nil {
		return nil, e
	}
	for _, row := range rows {
		safe := make([]string, len(row))
		for i, v := range row {
			safe[i] = Cell(v)
		}
		if e := w.Write(safe); e != nil {
			return nil, e
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}
func HTML(title string, headers []string, rows [][]string) ([]byte, error) {
	const page = `<!doctype html><html lang="en"><meta charset="utf-8"><title>{{.Title}}</title><style>body{font:14px system-ui;margin:32px;color:#172331}table{border-collapse:collapse;width:100%}td,th{border:1px solid #bbb;padding:8px;text-align:left;overflow-wrap:anywhere}th{background:#edf7f6}h1{font-size:24px}@media print{body{margin:0}tr{break-inside:avoid}}</style><h1>{{.Title}}</h1><p>SADPTools · Recorded data and observations. Unknown does not mean healthy; no response does not mean an address is free.</p><table><thead><tr>{{range .Headers}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody></table></html>`
	var b bytes.Buffer
	t, e := template.New("report").Parse(page)
	if e != nil {
		return nil, e
	}
	e = t.Execute(&b, struct {
		Title   string
		Headers []string
		Rows    [][]string
	}{title, headers, rows})
	return b.Bytes(), e
}
func Encode(format, title string, value any, headers []string, rows [][]string) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(value, "", "  ")
	case "csv":
		return CSV(headers, rows)
	case "html":
		return HTML(title, headers, rows)
	default:
		return nil, errors.New("choose JSON, CSV or HTML")
	}
}
