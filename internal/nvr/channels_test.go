package nvr

import (
	"encoding/json"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"strings"
	"testing"
)

func TestChannelFixturesUnknownStatusAndIdentity(t *testing.T) {
	for _, size := range []int{0, 4, 16, 32} {
		var b strings.Builder
		b.WriteString("<InputProxyChannelList>")
		for i := 0; i < size; i++ {
			fmt.Fprintf(&b, `<InputProxyChannel><id>%d</id><name>Duplicate name</name><sourceInputPortDescriptor><ipAddress>192.0.2.10</ipAddress><serialNumber>SYNTHETIC-%d</serialNumber><password>synthetic-secret</password></sourceInputPortDescriptor></InputProxyChannel>`, i+1, i)
		}
		b.WriteString("</InputProxyChannelList>")
		channels, e := ParseChannels([]byte(b.String()))
		if e != nil || len(channels) != size {
			t.Fatal(e, len(channels))
		}
		if size == 0 {
			continue
		}
		if channels[0].Status != "unknown" || channels[0].Recording != "unknown" {
			t.Fatal("missing status guessed")
		}
		e = ApplyStatus(channels, []byte(`<InputProxyChannelStatusList><InputProxyChannelStatus><id>1</id><online>false</online></InputProxyChannelStatus></InputProxyChannelStatusList>`))
		if e != nil || channels[0].Status != "offline" {
			t.Fatal(e)
		}
		Match(channels, []sites.Record{{ID: "camera", Device: devices.Device{Serial: "SYNTHETIC-0", IPv4: "192.0.2.99"}}, {ID: "other", Device: devices.Device{Serial: "different", IPv4: "192.0.2.10"}}})
		if channels[0].LinkedDeviceID != "camera" || channels[1].LinkedDeviceID != "" {
			t.Fatal("IP match overrode identity")
		}
		raw, _ := json.Marshal(channels)
		if strings.Contains(string(raw), "synthetic-secret") {
			t.Fatal("secret leaked")
		}
	}
}
func TestStreamMappingUsesReportedInput(t *testing.T) {
	streams, e := ParseStreams([]byte(`<StreamingChannelList><StreamingChannel><id>444</id><Video><videoInputChannelID>2</videoInputChannelID><videoCodecType>H264</videoCodecType><constantBitRate>2000</constantBitRate></Video></StreamingChannel></StreamingChannelList>`))
	if e != nil || streams[0].InputChannel != "2" {
		t.Fatal(e)
	}
	if _, e = ParseChannels([]byte(`<InputProxyChannelList><InputProxyChannel><id>1</id></InputProxyChannel><InputProxyChannel><id>1</id></InputProxyChannel></InputProxyChannelList>`)); e == nil {
		t.Fatal("duplicate channel accepted")
	}
}
