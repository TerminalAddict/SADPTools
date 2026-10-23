package switches

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/gosnmp/gosnmp"
	"net"
	"net/netip"
	"strings"
	"time"
)

// OIDs are fixed read-only fields from IF-MIB (RFC 2863), BRIDGE/Q-BRIDGE
// (RFC 4188/4363), POWER-ETHERNET (RFC 3621), LLDP and Cisco's published MIB.
const ifEntry = ".1.3.6.1.2.1.2.2.1"
const ifXEntry = ".1.3.6.1.2.1.31.1.1.1"
const bridgePorts = ".1.3.6.1.2.1.17.1.4.1.2"
const fdbEntry = ".1.3.6.1.2.1.17.4.3.1"
const qfdbEntry = ".1.3.6.1.2.1.17.7.1.2.2.1"
const vlanEntry = ".1.3.6.1.2.1.17.7.1.4.2.1"
const lldpLocal = ".1.0.8802.1.1.2.1.3.7.1"
const lldpRemote = ".1.0.8802.1.1.2.1.4.1.1"
const poeStatus = ".1.3.6.1.2.1.105.1.1.1.6"
const ciscoPower = ".1.3.6.1.4.1.9.9.402.1.2.1.9"
const ciscoPhysical = ".1.3.6.1.4.1.9.9.402.1.2.1.11"
const entityAlias = ".1.3.6.1.2.1.47.1.3.2.1.2"

var columns = []string{ifEntry + ".2", ifEntry + ".5", ifEntry + ".6", ifEntry + ".7", ifEntry + ".8", ifXEntry + ".1", ifXEntry + ".15", ifXEntry + ".18", bridgePorts, fdbEntry + ".2", fdbEntry + ".3", qfdbEntry + ".2", qfdbEntry + ".3", vlanEntry + ".3", vlanEntry + ".4", lldpLocal + ".2", lldpLocal + ".3", lldpLocal + ".4", lldpRemote + ".4", lldpRemote + ".5", lldpRemote + ".7", lldpRemote + ".9", lldpRemote + ".12", poeStatus}

type Walker interface {
	BulkWalk(string, gosnmp.WalkFunc) error
}

func client(ctx context.Context, a Asset, address string, secret vault.Secret) (*gosnmp.GoSNMP, error) {
	if e := a.Validate(); e != nil {
		return nil, e
	}
	if secret.Password == "" {
		return nil, errors.New("assign and unlock the switch credential profile")
	}
	c := &gosnmp.GoSNMP{Target: address, Port: uint16(a.Port), Transport: "udp4", Context: ctx, Timeout: 1200 * time.Millisecond, Retries: 0, MaxOids: 20, MaxRepetitions: 20, ContextName: a.ContextName}
	if a.Version == "2c" {
		if !a.AllowV2 {
			return nil, errors.New("SNMPv2c needs explicit permission")
		}
		c.Version = gosnmp.Version2c
		c.Community = secret.Password
	} else {
		if secret.Username == "" || len(secret.Password) < 8 || len(secret.PrivacyPassword) < 8 {
			return nil, errors.New("SNMPv3 needs a security name and authentication/privacy passwords of at least 8 bytes")
		}
		auth := gosnmp.SHA256
		switch a.AuthProtocol {
		case "sha1":
			auth = gosnmp.SHA
		case "sha512":
			auth = gosnmp.SHA512
		}
		c.Version = gosnmp.Version3
		c.MsgFlags = gosnmp.AuthPriv
		c.SecurityModel = gosnmp.UserSecurityModel
		c.SecurityParameters = &gosnmp.UsmSecurityParameters{UserName: secret.Username, AuthenticationProtocol: auth, AuthenticationPassphrase: secret.Password, PrivacyProtocol: gosnmp.AES, PrivacyPassphrase: secret.PrivacyPassword}
	}
	// Never attach a library debug logger: it may format authentication material.
	return c, nil
}
func Poll(parent context.Context, a Asset, secret vault.Secret, devices []devices.Device, progress func(string)) (Report, error) {
	defer secret.Clear()
	r := Report{AssetID: a.ID, SiteID: a.SiteID, SwitchName: a.Name, At: time.Now().UTC().Format(time.RFC3339Nano)}
	if e := a.Validate(); e != nil {
		return r, e
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	resolve, cancelResolve := context.WithTimeout(ctx, 3*time.Second)
	ips, e := net.DefaultResolver.LookupNetIP(resolve, "ip4", a.Host)
	cancelResolve()
	if e != nil || len(ips) == 0 {
		return r, errors.New("switch hostname/IPv4 could not be resolved")
	}
	ip := ips[0].Unmap()
	if !ip.Is4() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip == netip.MustParseAddr("255.255.255.255") {
		return r, errors.New("switch resolved to an unusable IPv4 address")
	}
	r.Address = ip.String()
	c, e := client(ctx, a, r.Address, secret)
	if e != nil {
		return r, e
	}
	defer func() {
		c.Community = ""
		if p, ok := c.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok {
			p.AuthenticationPassphrase = ""
			p.PrivacyPassphrase = ""
			clear(p.SecretKey)
			clear(p.PrivacyKey)
			clear(p.PrivacyParameters)
		}
	}()
	if e = c.Connect(); e != nil {
		return r, errors.New("SNMP connection failed; check address, credential and switch access rules")
	}
	defer c.Conn.Close()
	stop := context.AfterFunc(ctx, func() { c.Conn.Close() })
	defer stop()
	packet, e := c.Get([]string{".1.3.6.1.2.1.1.1.0", ".1.3.6.1.2.1.1.2.0", ".1.3.6.1.2.1.1.5.0"})
	if e != nil || packet == nil || packet.Error != gosnmp.NoError {
		return r, errors.New("SNMP read failed; check credentials, authentication algorithms, routing and read-only access")
	}
	systemOID := ""
	for _, v := range packet.Variables {
		switch v.Name {
		case ".1.3.6.1.2.1.1.1.0":
			r.Description = stringValue(v)
		case ".1.3.6.1.2.1.1.2.0":
			systemOID = stringValue(v)
		case ".1.3.6.1.2.1.1.5.0":
			r.SystemName = stringValue(v)
		}
	}
	rows, warnings := Collect(ctx, c, systemOID, progress)
	r = Decode(a, r, rows, devices)
	r.Warnings = append(r.Warnings, warnings...)
	r.Cancelled = ctx.Err() != nil
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	return r, nil
}
func Collect(ctx context.Context, w Walker, systemOID string, progress func(string)) (map[string]gosnmp.SnmpPDU, []string) {
	rows := map[string]gosnmp.SnmpPDU{}
	warnings := []string{}
	roots := append([]string{}, columns...)
	if len(systemOID) >= len(".1.3.6.1.4.1.9.") && systemOID[:len(".1.3.6.1.4.1.9.")] == ".1.3.6.1.4.1.9." {
		roots = append(roots, ciscoPower, ciscoPhysical, entityAlias)
	}
	for _, root := range roots {
		if ctx.Err() != nil {
			break
		}
		if progress != nil {
			progress("Reading SNMP " + root)
		}
		count := 0
		e := w.BulkWalk(root, func(p gosnmp.SnmpPDU) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			count++
			if count > 20000 || len(rows) >= 50000 {
				return errors.New("SNMP table limit reached")
			}
			if !strings.HasPrefix(p.Name, root+".") {
				return errors.New("SNMP returned an out-of-scope OID")
			}
			if len(p.Name) > 256 {
				return errors.New("invalid OID length")
			}
			if p.Type == gosnmp.NoSuchInstance || p.Type == gosnmp.NoSuchObject || p.Type == gosnmp.EndOfMibView {
				return nil
			}
			if bytes, ok := p.Value.([]byte); ok && len(bytes) > 4096 {
				return errors.New("SNMP value exceeds limit")
			}
			rows[p.Name] = p
			return nil
		})
		if e != nil {
			warnings = append(warnings, "Unsupported, incomplete or timed-out table "+root+"; related fields remain unknown")
		}
	}
	return rows, warnings
}
