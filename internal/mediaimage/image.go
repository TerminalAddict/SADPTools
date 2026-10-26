// Package mediaimage normalises user-selected images, discarding metadata and
// enforcing decoded-size limits before allocating a frame.
package mediaimage

import (
	"bytes"
	"errors"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	"image/png"
)

const MaxInput = 16 << 20
const MaxPixels = 16_000_000

func Normalise(data []byte) (out []byte, width, height int, err error) {
	if len(data) == 0 || len(data) > MaxInput {
		return nil, 0, 0, errors.New("image must be between 1 byte and 16 MiB")
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || (format != "png" && format != "jpeg" && format != "webp") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, 0, 0, errors.New("use a valid PNG, JPEG or WebP image up to 16 megapixels")
	}
	im, _, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, 0, 0, errors.New("image is incomplete or corrupt")
	}
	var b bytes.Buffer
	e = png.Encode(&b, im)
	if e != nil || b.Len() > MaxInput {
		return nil, 0, 0, errors.New("normalised image exceeds 16 MiB")
	}
	return b.Bytes(), cfg.Width, cfg.Height, nil
}
