package onvif

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

type ProbeMetadata struct {
	Code        int    `json:"code"`
	ContentType string `json:"contentType"`
}
type probeMetadataKey struct{}

// Inspect is intentionally not an arbitrary SOAP/URL interface. All parameter
// strings and service categories below are defined by this implementation.
func (c *Client) Inspect(ctx context.Context, operation string, s vault.Secret) ([]byte, ProbeMetadata, error) {
	var m ProbeMetadata
	ctx = context.WithValue(ctx, probeMetadataKey{}, &m)
	var data []byte
	var err error
	switch operation {
	case "GetServices":
		data, err = c.soap(ctx, c.target.URL, deviceNS, operation, "<tds:IncludeCapability>true</tds:IncludeCapability>", s, 0)
	case "GetCapabilities":
		data, err = c.soap(ctx, c.target.URL, deviceNS, operation, "<tds:Category>All</tds:Category>", s, 0)
	case "GetProfiles":
		endpoint, e := c.service(ctx, media1NS, "Media", s)
		if e != nil {
			return nil, m, e
		}
		data, err = c.soap(ctx, endpoint, media1NS, operation, "", s, 0)
	default:
		if !readOperations[operation] {
			return nil, m, errors.New("operation is not in the read-only inspector catalogue")
		}
		data, err = c.Read(ctx, operation, s)
	}
	return data, m, err
}
