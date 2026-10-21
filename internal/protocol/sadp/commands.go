// Command wire fields adapted from cameronnewman/hikvision-tooling commands.go.
// Copyright (c) 2025 Cameron Newman. MIT: LICENSES/hikvision-tooling-MIT.txt.
package sadp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

type Action string

const (
	Activate            Action = "activate"
	UpdateNetwork       Action = "update"
	ExchangeCode        Action = "exchangecode"
	EncryptionString    Action = "getencryptstring"
	EncryptionStringV31 Action = "getencryptstring_v31"
	ResetPassword       Action = "resetPassword"
	SecurityCode        Action = "securityCode"
)

func (a Action) ReadOnly() bool {
	return a == ExchangeCode || a == EncryptionString || a == EncryptionStringV31
}
func (a Action) Valid() bool {
	return a.ReadOnly() || a == Activate || a == UpdateNetwork || a == ResetPassword || a == SecurityCode
}

type NetworkSettings struct {
	IPv4        string `json:"ipv4"`
	SubnetMask  string `json:"subnetMask"`
	Gateway     string `json:"gateway"`
	CommandPort int    `json:"commandPort"`
	DHCP        bool   `json:"dhcp"`
}

// Command never interpolates credentials into templates. encoding/xml escapes them.
func Command(id, mac string, action Action, password, code string, network NetworkSettings) ([]byte, error) {
	if !action.Valid() || id == "" || mac == "" {
		return nil, errors.New("invalid SADP command identity or action")
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	e := xml.NewEncoder(&b)
	root := xml.StartElement{Name: xml.Name{Local: "Probe"}}
	if err := e.EncodeToken(root); err != nil {
		return nil, err
	}
	fields := [][2]string{{"Uuid", id}}
	if action == UpdateNetwork {
		fields = append(fields, [2]string{"Types", string(action)}, [2]string{"PWErrorParse", "true"}, [2]string{"MAC", mac})
	} else {
		fields = append(fields, [2]string{"MAC", mac}, [2]string{"Types", string(action)})
	}
	switch action {
	case ExchangeCode:
		fields = append(fields, [2]string{"Code", ""})
	case ResetPassword:
		fields = append(fields, [2]string{"Code", code})
	case SecurityCode:
		fields = append(fields, [2]string{"SecurityCode", code})
	}
	if !action.ReadOnly() {
		fields = append(fields, [2]string{"Password", password})
	}
	for _, f := range fields {
		if err := e.EncodeElement(f[1], xml.StartElement{Name: xml.Name{Local: f[0]}}); err != nil {
			return nil, errors.New("cannot encode SADP command")
		}
	}
	if action == UpdateNetwork {
		for _, f := range []struct {
			name  string
			value any
		}{{"IPv4Address", network.IPv4}, {"CommandPort", network.CommandPort}, {"IPv4SubnetMask", network.SubnetMask}, {"IPv4Gateway", network.Gateway}, {"DHCP", network.DHCP}} {
			if err := e.EncodeElement(f.value, xml.StartElement{Name: xml.Name{Local: f.name}}); err != nil {
				return nil, err
			}
		}
	}
	if err := e.EncodeToken(root.End()); err != nil {
		return nil, err
	}
	if err := e.Flush(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Only known fields are exposed. Never log raw replies: firmware may echo secrets.
type CommandReply struct{ UUID, MAC, Types, Result, ErrorCode, Code, EncryptString string }

// Reject ambiguous nested/duplicate fields and extra roots. Errors omit input text.
func ParseCommandReply(data []byte) (CommandReply, error) {
	bad := errors.New("malformed SADP command reply")
	if len(data) == 0 || len(data) > MaxPacketSize {
		return CommandReply{}, bad
	}
	data = bytes.TrimPrefix(bytes.TrimSpace(bytes.TrimRight(data, "\x00")), []byte{0xef, 0xbb, 0xbf})
	d := xml.NewDecoder(bytes.NewReader(data))
	fields := map[string]string{}
	depth, roots := 0, 0
	name := ""
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return CommandReply{}, bad
		}
		switch v := t.(type) {
		case xml.StartElement:
			depth++
			if v.Name.Space != "" || len(v.Attr) != 0 {
				return CommandReply{}, bad
			}
			if depth == 1 {
				roots++
				if roots != 1 || v.Name.Local != "ProbeMatch" {
					return CommandReply{}, bad
				}
			} else if depth == 2 {
				name = v.Name.Local
				if _, exists := fields[name]; exists {
					return CommandReply{}, bad
				}
				fields[name] = ""
			} else {
				return CommandReply{}, bad
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 2 {
				fields[name] += string(v)
			} else if len(bytes.TrimSpace(v)) != 0 {
				return CommandReply{}, bad
			}
		case xml.Directive:
			return CommandReply{}, bad
		case xml.ProcInst:
			if roots != 0 || v.Target != "xml" {
				return CommandReply{}, bad
			}
		}
	}
	if roots != 1 || depth != 0 {
		return CommandReply{}, bad
	}
	get := func(key string) string { return strings.TrimSpace(fields[key]) }
	return CommandReply{UUID: get("Uuid"), MAC: get("MAC"), Types: get("Types"), Result: get("Result"), ErrorCode: get("ErrorCode"), Code: get("Code"), EncryptString: get("EncryptString")}, nil
}
