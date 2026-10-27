//go:build !windows && !linux && !darwin

package ipscan

import "errors"

func neighbours() ([]Neighbour, error) {
	return nil, errors.New("neighbour inspection is unsupported on this platform")
}
