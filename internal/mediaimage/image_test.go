package mediaimage

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestNormaliseBounds(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 10, 20)))
	out, w, h, e := Normalise(b.Bytes())
	if e != nil || w != 10 || h != 20 || len(out) == 0 {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{[]byte("<svg onload='x'/>"), b.Bytes()[:15], make([]byte, MaxInput+1)} {
		if _, _, _, e = Normalise(bad); e == nil {
			t.Fatal("unsafe image accepted")
		}
	}
}
