//go:build !linux && !windows && !darwin

package platform

import (
	"context"
	"errors"
)

func DestinationRoute(context.Context, string, string) (RouteDetail, error) {
	return RouteDetail{}, errors.New("destination route details unsupported on this platform")
}
