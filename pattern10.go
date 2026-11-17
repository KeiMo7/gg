package gg

import (
	"image"
	"image/color"

	"github.com/golang/freetype/raster"
)

type patternPainter10 struct {
	im     *Image10
	mask   *image.Alpha16
	p      Pattern
	dither DitherMode
}

func (r *patternPainter10) Paint(ss []raster.Span, done bool) {
	b := r.im.Bounds()
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			return
		}
		if s.X0 < b.Min.X {
			s.X0 = b.Min.X
		}
		if s.X1 > b.Max.X {
			s.X1 = b.Max.X
		}
		if s.X0 >= s.X1 {
			continue
		}
		const m = uint64(65535)
		y := s.Y - r.im.Rect.Min.Y
		x0 := s.X0 - r.im.Rect.Min.X
		i0 := r.im.PixOffset(s.X0, s.Y)
		i1 := i0 + (s.X1-s.X0)*4
		for i, x := i0, x0; i < i1; i, x = i+4, x+1 {
			ma := uint64(s.Alpha)
			if r.mask != nil {
				ma = ma * uint64(r.mask.Alpha16At(x, y).A) / m
				if ma == 0 {
					continue
				}
			}
			c := colorAt16(r.p, x, y)
			dr := uint32(expand10To16(r.im.Pix[i+0]))
			dg := uint32(expand10To16(r.im.Pix[i+1]))
			db := uint32(expand10To16(r.im.Pix[i+2]))
			da := uint32(expand10To16(r.im.Pix[i+3]))
			sa := uint64(c.A) * ma / m
			r.im.Pix[i+0] = quantize16To10(blend16(dr, uint32(c.R), sa, ma, m), r.dither, x, y)
			r.im.Pix[i+1] = quantize16To10(blend16(dg, uint32(c.G), sa, ma, m), r.dither, x, y)
			r.im.Pix[i+2] = quantize16To10(blend16(db, uint32(c.B), sa, ma, m), r.dither, x, y)
			r.im.Pix[i+3] = quantize16To10(blend16(da, uint32(c.A), sa, ma, m), r.dither, x, y)
		}
	}
}

func colorAt16(p Pattern, x, y int) color.RGBA64 {
	if high, ok := p.(HighDepthPattern); ok {
		return high.ColorAt16(x, y)
	}
	return color.RGBA64Model.Convert(p.ColorAt(x, y)).(color.RGBA64)
}

func blend16(dst, src uint32, sourceAlpha, maskAlpha, m uint64) uint32 {
	out := (uint64(dst)*uint64(65535-sourceAlpha) + uint64(src)*maskAlpha) / m
	if out > 65535 {
		return 65535
	}
	return uint32(out)
}

func newPatternPainter10(im *Image10, mask *image.Alpha16, p Pattern, dither DitherMode) *patternPainter10 {
	return &patternPainter10{im: im, mask: mask, p: p, dither: dither}
}

type alphaPainter16 struct {
	im *image.Alpha16
}

func (p *alphaPainter16) Paint(ss []raster.Span, done bool) {
	b := p.im.Bounds()
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			return
		}
		if s.X0 < b.Min.X {
			s.X0 = b.Min.X
		}
		if s.X1 > b.Max.X {
			s.X1 = b.Max.X
		}
		if s.X0 >= s.X1 {
			continue
		}
		i0 := (s.Y-b.Min.Y)*p.im.Stride + (s.X0-b.Min.X)*2
		i1 := i0 + (s.X1-s.X0)*2
		for i := i0; i < i1; i += 2 {
			dst := uint32(p.im.Pix[i])<<8 | uint32(p.im.Pix[i+1])
			src := s.Alpha
			out := uint32(src) + dst*(65535-src)/65535
			if out > 65535 {
				out = 65535
			}
			p.im.Pix[i+0] = uint8(out >> 8)
			p.im.Pix[i+1] = uint8(out)
		}
	}
}

func newAlphaPainter16(im *image.Alpha16) *alphaPainter16 {
	return &alphaPainter16{im: im}
}

func combineAlpha16(dst, src *image.Alpha16) {
	b := dst.Bounds().Intersect(src.Bounds())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := uint64(dst.Alpha16At(x, y).A)
			s := uint64(src.Alpha16At(x, y).A)
			dst.SetAlpha16(x, y, color.Alpha16{A: uint16(d * s / 65535)})
		}
	}
}
