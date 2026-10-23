// Package onvif implements scoped WS-Discovery and read-only ONVIF queries.
package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"golang.org/x/net/ipv4"
	"io"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DiscoveryNS = "http://schemas.xmlsoap.org/ws/2005/04/discovery"

func Probe(id string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="` + DiscoveryNS + `" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><s:Header><a:MessageID>` + xmlText(id) + `</a:MessageID><a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To><a:Action>` + DiscoveryNS + `/Probe</a:Action></s:Header><s:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></s:Body></s:Envelope>`)
}
func xmlText(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
func ValidXAddr(raw, address string) bool {
	if len(raw) > 2048 {
		return false
	}
	u, e := url.Parse(raw)
	ip, iperr := netip.ParseAddr(address)
	if e != nil || iperr != nil || !ip.Is4() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.String() == "255.255.255.255" {
		return false
	}
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil || p < 1 || p > 65535 {
			return false
		}
	}
	return u.Hostname() == address && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.RawPath == "" && (strings.HasPrefix(strings.ToLower(u.Path), "/onvif/") || strings.EqualFold(u.Path, "/onvif")) && !strings.Contains(u.Path, "..")
}
func Parse(data []byte, source, requestID string, adapter platform.Adapter) ([]devices.Device, error) {
	if len(data) > 64<<10 {
		return nil, errors.New("oversized WS-Discovery response")
	}
	var doc struct {
		XMLName xml.Name `xml:"Envelope"`
		Header  struct {
			RelatesTo string `xml:"RelatesTo"`
		} `xml:"Header"`
		Body struct {
			Matches struct {
				XMLName xml.Name `xml:"ProbeMatches"`
				Devices []struct {
					Address string `xml:"EndpointReference>Address"`
					Scopes  string `xml:"Scopes"`
					XAddrs  string `xml:"XAddrs"`
				} `xml:"ProbeMatch"`
			} `xml:"ProbeMatches"`
		} `xml:"Body"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if e := decoder.Decode(&doc); e != nil {
		return nil, errors.New("malformed WS-Discovery XML")
	}
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, errors.New("malformed trailing XML")
		}
		if text, ok := token.(xml.CharData); !ok || len(bytes.TrimSpace(text)) > 0 {
			return nil, errors.New("multiple XML documents")
		}
	}
	if doc.XMLName.Space != "http://www.w3.org/2003/05/soap-envelope" || doc.Body.Matches.XMLName.Space != DiscoveryNS || strings.TrimSpace(doc.Header.RelatesTo) != requestID || len(doc.Body.Matches.Devices) > 256 {
		return nil, errors.New("unrelated WS-Discovery response")
	}
	out := []devices.Device{}
	for _, r := range doc.Body.Matches.Devices {
		uuid := strings.TrimSpace(r.Address)
		if uuid == "" || len(uuid) > 512 {
			continue
		}
		d := devices.Device{UUID: uuid, IPv4: source, Protocols: []string{"onvif"}, Capabilities: []string{"onvif-device-information"}, XAddrs: []string{}, Interfaces: []devices.Interface{{Index: adapter.Index, Name: adapter.Name, Address: adapter.Address()}}}
		for _, raw := range strings.Fields(r.XAddrs) {
			if ValidXAddr(raw, source) {
				d.XAddrs = append(d.XAddrs, raw)
			}
		}
		if len(d.XAddrs) == 0 {
			continue
		}
		for _, raw := range strings.Fields(r.Scopes) {
			u, e := url.Parse(raw)
			if e != nil || u.Scheme != "onvif" || u.Host != "www.onvif.org" {
				continue
			}
			parts := strings.SplitN(strings.TrimPrefix(u.EscapedPath(), "/"), "/", 2)
			if len(parts) != 2 {
				continue
			}
			value, e := url.PathUnescape(parts[1])
			if e != nil || len(value) > 512 {
				continue
			}
			switch parts[0] {
			case "name":
				d.Name = value
			case "hardware":
				d.Model = value
			}
		}
		d.ID = d.Key()
		out = append(out, d)
	}
	return out, nil
}
func Scan(parent context.Context, opts discovery.Options, onDevice func(devices.Device)) (discovery.Result, error) {
	result := discovery.Result{Devices: []devices.Device{}, Warnings: []string{}, Started: time.Now()}
	if opts.Timeout <= 0 || opts.Timeout > 5*time.Minute {
		return result, errors.New("ONVIF timeout must be positive and at most 5 minutes")
	}
	all, e := platform.Interfaces()
	if e != nil {
		return result, e
	}
	adapters, e := platform.Select(all, opts.Interface)
	if e != nil {
		return result, e
	}
	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()
	inventory := devices.NewInventory()
	var mu sync.Mutex
	var wg sync.WaitGroup
	warn := func(s string) { mu.Lock(); result.Warnings = append(result.Warnings, s); mu.Unlock() }
	for _, a := range adapters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(a.Address())})
			if e != nil {
				warn(a.Name + ": ONVIF socket unavailable")
				return
			}
			defer conn.Close()
			stop := context.AfterFunc(ctx, func() { conn.Close() })
			defer stop()
			p := ipv4.NewPacketConn(conn)
			iface, e := net.InterfaceByIndex(a.Index)
			if e != nil {
				warn(a.Name + ": adapter disappeared")
				return
			}
			if p.SetMulticastInterface(iface) != nil || p.SetMulticastTTL(1) != nil {
				warn(a.Name + ": ONVIF multicast unavailable")
				return
			}
			nonce := make([]byte, 16)
			rand.Read(nonce)
			nonce[6] = (nonce[6] & 0x0f) | 0x40
			nonce[8] = (nonce[8] & 0x3f) | 0x80
			id := fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", nonce[:4], nonce[4:6], nonce[6:8], nonce[8:10], nonce[10:])
			payload := Probe(id)
			if _, e = conn.WriteToUDP(payload, &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 3702}); e != nil {
				warn(a.Name + ": ONVIF probe send failed")
				return
			}
			buf := make([]byte, 64<<10)
			for ctx.Err() == nil {
				conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
				n, peer, e := conn.ReadFromUDP(buf)
				if e != nil {
					if timeout, ok := e.(net.Error); ok && timeout.Timeout() {
						continue
					}
					break
				}
				rows, e := Parse(buf[:n], peer.IP.String(), id, a)
				if e != nil {
					continue
				}
				for _, d := range rows {
					merged, _ := inventory.Upsert(d, time.Now())
					if onDevice != nil {
						onDevice(merged)
					}
				}
			}
		}()
	}
	wg.Wait()
	result.Devices = inventory.Snapshot()
	result.Finished = time.Now()
	if parent.Err() != nil {
		return result, parent.Err()
	}
	return result, nil
}
