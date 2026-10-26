package xmlsafe

import (
	"strings"
	"testing"
)

func TestBoundedXMLRejectsEntitiesDepthAndTrailingRoots(t *testing.T) {
	for _, raw := range []string{"<!DOCTYPE a [<!ENTITY x SYSTEM 'file:///etc/passwd'>]><a>&x;</a>", strings.Repeat("<a>", 40) + strings.Repeat("</a>", 40), "<a/><b/>", "<broken"} {
		if _, e := Parse([]byte(raw)); e == nil {
			t.Fatal("unsafe XML accepted")
		}
	}
	n, e := Parse([]byte(`<a xmlns="urn:fixture"><b test="safe">fixture</b></a>`))
	if e != nil || n.Value("b") != "fixture" || n.At("missing").Value("x") != "" {
		t.Fatal(n, e)
	}
}
