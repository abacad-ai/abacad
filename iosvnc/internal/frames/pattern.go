package frames

import (
	"context"
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"iosvnc/internal/rfbserver"
)

// Pattern is a synthetic 60 Hz source. It exists so the RFB side can be tested
// and demonstrated without a device attached.
type Pattern struct {
	w, h int

	mu    sync.Mutex
	cur   rfbserver.Frame
	wait  chan struct{}
	seq   uint64
	frame *image.RGBA
}

// NewPattern returns a source of the given size.
func NewPattern(w, h int) *Pattern {
	p := &Pattern{w: w, h: h, wait: make(chan struct{}), frame: image.NewRGBA(image.Rect(0, 0, w, h))}
	p.paint(0)
	p.seq = 1
	p.cur = rfbserver.Frame{Image: p.frame, Seq: 1}
	return p
}

// Size reports the synthetic framebuffer size.
func (p *Pattern) Size() (int, int) { return p.w, p.h }

// Run repaints at 60 Hz until ctx is cancelled.
func (p *Pattern) Run(ctx context.Context) {
	t := time.NewTicker(time.Second / 60)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.paint(now.Sub(start).Seconds())
			p.mu.Lock()
			p.seq++
			p.cur = rfbserver.Frame{Image: p.frame, Seq: p.seq}
			close(p.wait)
			p.wait = make(chan struct{})
			p.mu.Unlock()
		}
	}
}

// Wait blocks until a frame newer than after exists.
func (p *Pattern) Wait(ctx context.Context, after uint64) (rfbserver.Frame, error) {
	for {
		p.mu.Lock()
		if p.cur.Seq > after {
			f := p.cur
			p.mu.Unlock()
			return f, nil
		}
		ch := p.wait
		p.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return rfbserver.Frame{}, ctx.Err()
		}
	}
}

// paint draws colour bars, a moving marker and a brightening background, all of
// which make a torn or stale frame obvious on the viewer.
func (p *Pattern) paint(t float64) {
	bars := []color.RGBA{
		{255, 255, 255, 255}, {255, 255, 0, 255}, {0, 255, 255, 255}, {0, 255, 0, 255},
		{255, 0, 255, 255}, {255, 0, 0, 255}, {0, 0, 255, 255}, {16, 16, 16, 255},
	}
	barW := p.w / len(bars)
	for y := 0; y < p.h; y++ {
		row := p.frame.Pix[y*p.frame.Stride:]
		for x := 0; x < p.w; x++ {
			i := x * 4
			if y < p.h/3 {
				bi := x / barW
				if bi >= len(bars) {
					bi = len(bars) - 1
				}
				c := bars[bi]
				row[i], row[i+1], row[i+2], row[i+3] = c.R, c.G, c.B, 255
				continue
			}
			// A slow gradient in the lower two thirds.
			v := uint8(16 + 200*(float64(x)/float64(p.w)))
			row[i], row[i+1], row[i+2], row[i+3] = v, v/2, 255-v, 255
		}
	}
	// Marker sweeps horizontally, wrapping once per two seconds.
	span := p.h - p.h/3
	my := p.h/3 + int(float64(span)*(0.5+0.5*math.Sin(2*math.Pi*t/2)))
	mx := int(float64(p.w) * (0.5 + 0.5*math.Sin(2*math.Pi*t/4)))
	for dy := -40; dy <= 40; dy++ {
		y := my + dy
		if y < 0 || y >= p.h {
			continue
		}
		row := p.frame.Pix[y*p.frame.Stride:]
		for dx := -40; dx <= 40; dx++ {
			x := mx + dx
			if x < 0 || x >= p.w {
				continue
			}
			i := x * 4
			row[i], row[i+1], row[i+2] = 255, 255, 255
		}
	}
}
