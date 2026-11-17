package gg

import (
	"image"
	"image/color"
)

// Image10 is a 10-bit RGBA image. Samples are stored in a 16-bit container,
// but every channel is constrained to [0, 1023]. RGBA samples are
// alpha-premultiplied, matching image.RGBA semantics.
type Image10 struct {
	Pix    []uint16
	Stride int
	Rect   image.Rectangle
}

// NewImage10 creates a 10-bit image with the specified bounds.
func NewImage10(r image.Rectangle) *Image10 {
	w, h := r.Dx(), r.Dy()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Image10{
		Pix:    make([]uint16, w*h*4),
		Stride: w * 4,
		Rect:   r,
	}
}

// NewImage10Rect creates a 10-bit image with bounds (0, 0, width, height).
func NewImage10Rect(width, height int) *Image10 {
	return NewImage10(image.Rect(0, 0, width, height))
}

// BitDepth returns the effective channel precision.
func (p *Image10) BitDepth() int {
	return BitDepth10
}

// ColorModel returns color.RGBA64Model so standard encoders select a 16-bit
// output representation for the 10-bit samples.
func (p *Image10) ColorModel() color.Model {
	return color.RGBA64Model
}

// Bounds returns the image bounds.
func (p *Image10) Bounds() image.Rectangle {
	return p.Rect
}

// PixOffset returns the index of the first sample for pixel (x, y).
func (p *Image10) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*4
}

// RGBA10At returns the 10-bit alpha-premultiplied channels at (x, y).
func (p *Image10) RGBA10At(x, y int) (r, g, b, a uint16) {
	if !(image.Point{X: x, Y: y}.In(p.Rect)) {
		return 0, 0, 0, 0
	}
	i := p.PixOffset(x, y)
	return p.Pix[i], p.Pix[i+1], p.Pix[i+2], p.Pix[i+3]
}

// RGBA64At returns a 16-bit alpha-premultiplied representation of the pixel.
func (p *Image10) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.RGBA10At(x, y)
	return color.RGBA64{
		R: expand10To16(r),
		G: expand10To16(g),
		B: expand10To16(b),
		A: expand10To16(a),
	}
}

// At returns the pixel at (x, y).
func (p *Image10) At(x, y int) color.Color {
	return p.RGBA64At(x, y)
}

// SetRGBA10 writes an exact 10-bit pixel. Values above 1023 are clamped.
func (p *Image10) SetRGBA10(x, y int, r, g, b, a uint16) {
	if !(image.Point{X: x, Y: y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	p.Pix[i+0] = clamp10(r)
	p.Pix[i+1] = clamp10(g)
	p.Pix[i+2] = clamp10(b)
	p.Pix[i+3] = clamp10(a)
}

// SetRGBA64 quantizes a 16-bit alpha-premultiplied color and writes it.
func (p *Image10) SetRGBA64(x, y int, c color.RGBA64) {
	if !(image.Point{X: x, Y: y}.In(p.Rect)) {
		return
	}
	p.SetRGBA10(x, y,
		quantize16To10(uint32(c.R), DitherNone, x, y),
		quantize16To10(uint32(c.G), DitherNone, x, y),
		quantize16To10(uint32(c.B), DitherNone, x, y),
		quantize16To10(uint32(c.A), DitherNone, x, y),
	)
}

// Set implements image/draw.Image.
func (p *Image10) Set(x, y int, c color.Color) {
	if !(image.Point{X: x, Y: y}.In(p.Rect)) {
		return
	}
	r, g, b, a := c.RGBA()
	p.SetRGBA10(x, y,
		quantize16To10(r, DitherNone, x, y),
		quantize16To10(g, DitherNone, x, y),
		quantize16To10(b, DitherNone, x, y),
		quantize16To10(a, DitherNone, x, y),
	)
}

// Opaque reports whether every pixel is fully opaque.
func (p *Image10) Opaque() bool {
	for i := 3; i < len(p.Pix); i += 4 {
		if p.Pix[i] != 1023 {
			return false
		}
	}
	return true
}

// SubImage returns a view sharing the same backing storage.
func (p *Image10) SubImage(r image.Rectangle) image.Image {
	r = r.Intersect(p.Rect)
	if r.Empty() {
		return &Image10{Rect: r}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &Image10{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

func copyImage10(src *Image10) *Image10 {
	dst := NewImage10(src.Rect)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		srcOffset := src.PixOffset(src.Rect.Min.X, y)
		dstOffset := dst.PixOffset(dst.Rect.Min.X, y)
		copy(dst.Pix[dstOffset:dstOffset+dst.Rect.Dx()*4],
			src.Pix[srcOffset:srcOffset+src.Rect.Dx()*4])
	}
	return dst
}

func imageToImage10(src image.Image, dither DitherMode) *Image10 {
	dst := NewImage10(src.Bounds())
	for y := dst.Rect.Min.Y; y < dst.Rect.Max.Y; y++ {
		for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			dst.SetRGBA10(x, y,
				quantize16To10(r, dither, x, y),
				quantize16To10(g, dither, x, y),
				quantize16To10(b, dither, x, y),
				quantize16To10(a, dither, x, y),
			)
		}
	}
	return dst
}

func expand10To16(v uint16) uint16 {
	return v<<6 | v>>4
}

func clamp10(v uint16) uint16 {
	if v > 1023 {
		return 1023
	}
	return v
}

func quantize16To10(v uint32, dither DitherMode, x, y int) uint16 {
	q := uint16(v >> 6)
	if expand10To16(q) == uint16(v) {
		return q
	}
	if q < 1023 && expand10To16(q+1) == uint16(v) {
		return q + 1
	}
	if fits8BitSample(v) {
		x8 := uint16(uint8(v))
		return x8<<2 | x8>>6
	}
	nearestQ := (v + 32) >> 6
	if nearestQ > 1023 {
		nearestQ = 1023
	}
	nearest := uint16(nearestQ)
	if dither == DitherOrdered {
		nearestQ = (v + uint32(bayer8[y&7][x&7])) >> 6
		if nearestQ > 1023 {
			nearestQ = 1023
		}
		return uint16(nearestQ)
	}
	return nearest
}

var bayer8 = [8][8]uint8{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}
