package isapi

import (
	"context"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Generated protocol fixture; not a device capture.
const syntheticDeviceInfo = `<DeviceInfo xmlns="http://www.isapi.org/ver20/XMLSchema"><deviceName>SYNTHETIC camera</deviceName><model>SYNTHETIC-MODEL</model><serialNumber>SYNTHETIC-001</serialNumber><firmwareVersion>synthetic 1.0</firmwareVersion><macAddress>02-00-00-00-00-10</macAddress><deviceType>IPCamera</deviceType><password>DO-NOT-COPY</password></DeviceInfo>`

func TestParseDeviceInformation(t *testing.T) {
	d, e := parseDeviceInformation([]byte(syntheticDeviceInfo))
	if e != nil || d.Model != "SYNTHETIC-MODEL" || d.MAC != "02:00:00:00:00:10" || d.Serial != "SYNTHETIC-001" || d.Vendor != "" || d.Activated != nil || d.DHCP != nil || d.IPv4 != "" {
		t.Fatal(d, e)
	}
	for _, bad := range []string{"", "<DeviceInfo>", "<html>Login</html>", "<DeviceInfo/>", "<DeviceInfo><macAddress>invalid</macAddress></DeviceInfo>", "<DeviceInfo><deviceName>Name alone is not identity</deviceName></DeviceInfo>"} {
		if _, e := parseDeviceInformation([]byte(bad)); e == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
func TestDeviceInformationReadOnlyEndpoint(t *testing.T) {
	c, e := New("192.0.2.10", "http://192.0.2.10:8080/", "")
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	c.http.Transport = syntheticTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.String() != "http://192.0.2.10:8080/ISAPI/System/deviceInfo" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected request")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(syntheticDeviceInfo))}, nil
	})
	d, e := c.DeviceInformation(context.Background(), vault.Secret{})
	if e != nil || d.Model != "SYNTHETIC-MODEL" || calls != 1 {
		t.Fatal(d, e, calls)
	}
}
