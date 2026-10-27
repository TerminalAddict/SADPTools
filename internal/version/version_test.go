package version

import "testing"

func TestDisplayVersionUnwrapsReleaseStampAndPreservesDirectBuilds(t *testing.T) {
	for _, v := range []string{"1.0.0", "1.0.1-beta.1", "1.0.1-rc.1", "1.0.1-dev"} {
		if displayVersion(v) != v || displayVersion("SADPToolsVersion["+v+"]") != v {
			t.Fatalf("displayed version differs from build version %s", v)
		}
	}
}
