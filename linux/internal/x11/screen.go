package x11

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/jezek/xgb/xproto"
)

// Shot is one captured frame: dimensions plus base64-encoded JPEG bytes. The
// wire field is named png_base64 for cross-platform compatibility even though
// the bytes are JPEG — same convention as the macOS and Android clients.
type Shot struct {
	W, H   int
	Base64 string
}

// jpegQuality trades size for fidelity on the multi-MB frames sent as base64.
const jpegQuality = 80

// Capture grabs the whole primary screen via GetImage (ZPixmap) and encodes it
// as JPEG. The root-window pixel space matches the coordinate space input verbs
// act in, so a captured pixel maps straight to a click point.
func (c *Conn) Capture() (Shot, error) {
	// Ask the server how big the root is right now rather than trusting the
	// connect-time snapshot — see refreshGeometry for why it goes stale.
	c.refreshGeometry()
	if c.width <= 0 || c.height <= 0 {
		return Shot{}, fmt.Errorf("screen has zero geometry (%dx%d)", c.width, c.height)
	}
	shot, err := c.captureSize(c.width, c.height)
	if err != nil {
		// A resize landing between the geometry read and the capture loses the
		// race. Re-read and try once more; a second failure is real.
		c.refreshGeometry()
		if c.width <= 0 || c.height <= 0 {
			return Shot{}, err
		}
		shot, err = c.captureSize(c.width, c.height)
	}
	return shot, err
}

// captureSize grabs a w×h rectangle from the root origin and encodes it. The
// dimensions are passed in so the request, the decode, and the reported size all
// describe one frame even if the screen resizes mid-capture.
func (c *Conn) captureSize(w, h int) (Shot, error) {
	reply, err := xproto.GetImage(
		c.c, xproto.ImageFormatZPixmap, xproto.Drawable(c.root),
		0, 0, uint16(w), uint16(h), 0xffffffff,
	).Reply()
	if err != nil {
		// Name the geometry we asked for: a BadMatch here almost always means the
		// rectangle no longer fits the root, and the numbers are the evidence.
		return Shot{}, fmt.Errorf("GetImage %dx%d: %w", w, h, err)
	}
	img, err := decodeZPixmap(reply.Data, w, h)
	if err != nil {
		return Shot{}, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Shot{}, fmt.Errorf("jpeg encode: %w", err)
	}
	return Shot{W: w, H: h, Base64: base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// decodeZPixmap turns raw ZPixmap bytes into an RGBA image. Depth-24 TrueColor
// visuals (the common case, incl. Xvfb) deliver 4 bytes/pixel little-endian as
// B,G,R,pad; 3 bytes/pixel as B,G,R. Both are swizzled to RGBA.
func decodeZPixmap(data []byte, w, h int) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	px := w * h
	if px == 0 {
		return img, nil
	}
	bpp := len(data) / px
	switch {
	case bpp >= 4:
		for p := 0; p < px; p++ {
			si, di := p*bpp, p*4
			img.Pix[di] = data[si+2]   // R
			img.Pix[di+1] = data[si+1] // G
			img.Pix[di+2] = data[si]   // B
			img.Pix[di+3] = 255
		}
	case bpp == 3:
		for p := 0; p < px; p++ {
			si, di := p*3, p*4
			img.Pix[di] = data[si+2]
			img.Pix[di+1] = data[si+1]
			img.Pix[di+2] = data[si]
			img.Pix[di+3] = 255
		}
	default:
		return nil, fmt.Errorf("unsupported ZPixmap depth: %d bytes/pixel", bpp)
	}
	return img, nil
}
