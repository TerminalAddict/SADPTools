package toolkit

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/sites"
)

func TestSiteImportPreviewCancelExpiryCommitAndClose(t *testing.T) {
	store, err := sites.Open(filepath.Join(t.TempDir(), "sites.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: store}
	defer m.Close()
	data := []byte(`{"format":"sadptools-site","version":1,"site":{"name":"Synthetic import"},"cameras":[{"name":"Fixture","ipv4":"192.0.2.1"}]}`)
	prepare := func() sites.ImportPreview {
		t.Helper()
		p, err := m.PrepareSiteImport(data, "")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := prepare()
	if list, _ := store.Sites(); len(list) != 0 {
		t.Fatal("preview wrote a site")
	}
	if _, err = m.CommitSiteImport("wrong-token", "Wrong"); err == nil {
		t.Fatal("wrong token accepted")
	}
	m.CancelSiteImport(p.Token)
	if m.siteImport != nil {
		t.Fatal("cancel retained import")
	}
	if _, err = m.CommitSiteImport(p.Token, "Cancelled"); err == nil {
		t.Fatal("cancelled preview accepted")
	}
	p = prepare()
	m.siteImport.expires = time.Now().Add(-time.Second)
	if _, err = m.CommitSiteImport(p.Token, "Expired"); err == nil || m.siteImport != nil {
		t.Fatal("expired preview accepted or retained")
	}
	p = prepare()
	old := p.Token
	p = prepare()
	if _, err = m.CommitSiteImport(old, "Replaced"); err == nil {
		t.Fatal("replaced preview accepted")
	}
	if _, err = m.CommitSiteImport(p.Token, ""); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err = m.CommitSiteImport(p.Token, "Imported fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.CommitSiteImport(p.Token, "Replay"); err == nil {
		t.Fatal("preview replay accepted")
	}
	if list, _ := store.Sites(); len(list) != 1 {
		t.Fatal("import did not commit once")
	}
	p = prepare()
	m.Close()
	if m.siteImport != nil {
		t.Fatal("close retained pending credentials")
	}
	if _, err = m.PrepareSiteImport(data, ""); err == nil {
		t.Fatal("closed manager accepted preview")
	}
}
