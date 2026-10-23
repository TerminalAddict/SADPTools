package reports

import (
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"strings"
	"testing"
)

func TestReportEscapingAndSecretBoundary(t *testing.T) {
	rows := [][]string{{" =HYPERLINK(\"synthetic\")", "<img src=x onerror=alert(1)>"}}
	csv, e := CSV([]string{"Name", "Evidence"}, rows)
	if e != nil || !strings.Contains(string(csv), "' =HYPERLINK") {
		t.Fatal("formula not escaped", e)
	}
	html, e := HTML("<script>fixture</script>", []string{"Name", "Evidence"}, rows)
	if e != nil || strings.Contains(string(html), "<img") || strings.Contains(string(html), "<script>") {
		t.Fatal("unsafe HTML", e)
	}
	json, e := Encode("json", "fixture", vault.Secret{Password: "Synthetic secret"}, nil, nil)
	if e != nil || strings.Contains(string(json), "Synthetic") {
		t.Fatal("secret serialized")
	}
}
