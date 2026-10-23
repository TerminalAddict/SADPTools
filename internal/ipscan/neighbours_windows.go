//go:build windows

package ipscan

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"unsafe"
)

type neighbourRow struct {
	Address      [28]byte
	Index        uint32
	Luid         uint64
	Physical     [32]byte
	Length       uint32
	State        uint32
	Flags        uint8
	_            [3]byte
	Reachability uint32
}

func neighbours() ([]Neighbour, error) {
	var table unsafe.Pointer
	r, _, _ := iphelper.NewProc("GetIpNetTable2").Call(windows.AF_INET, uintptr(unsafe.Pointer(&table)))
	if r != 0 || table == nil {
		return nil, errors.New("Windows neighbour table unavailable")
	}
	defer iphelper.NewProc("FreeMibTable").Call(uintptr(table))
	n := *(*uint32)(table)
	if n > 65536 {
		return nil, errors.New("neighbour table exceeds bound")
	}
	out := []Neighbour{}
	for _, row := range unsafe.Slice((*neighbourRow)(unsafe.Add(table, 8)), int(n)) {
		if row.Length != 6 || row.State < 2 {
			continue
		}
		iface, e := net.InterfaceByIndex(int(row.Index))
		if e != nil {
			continue
		}
		out = append(out, Neighbour{IP: net.IP(row.Address[4:8]).String(), MAC: net.HardwareAddr(row.Physical[:6]).String(), Interface: iface.Name, Reachable: row.State == 5, Source: "Windows neighbour cache"})
	}
	return out, nil
}
