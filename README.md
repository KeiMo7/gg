# Go Graphics

`gg` is a library for rendering 2D graphics in pure Go.

![Stars](http://i.imgur.com/CylQIJt.png)

## Installation

    go get -u github.com/fogleman/gg

Alternatively, you may use gopkg.in to grab a specific major-version:

    go get -u gopkg.in/fogleman/gg.v1

## Documentation

- godoc: https://godoc.org/github.com/fogleman/gg
- pkg.go.dev: https://pkg.go.dev/github.com/fogleman/gg?tab=doc

## Hello, Circle!

Look how easy!

```go
package main

import "github.com/fogleman/gg"

func main() {
    dc := gg.NewContext(1000, 1000)
    dc.DrawCircle(500, 500, 400)
    dc.SetRGB(0, 0, 0)
    dc.Fill()
    dc.SavePNG("out.png")
}
```

## Examples

There are [lots of examples](https://github.com/fogleman/gg/tree/master/examples) included. They're mostly for testing the code, but they're good for learning, too.

![Examples](http://i.imgur.com/tMFoyzu.png)

## Creating Contexts

There are a few ways of creating a context.

```go
NewContext(width, height int) *Context
NewContextForImage(im image.Image) *Context
NewContextForRGBA(im *image.RGBA) *Context
NewContextWithOptions(width, height int, opts RenderOptions) (*Context, error)
NewContextForImageWithOptions(im image.Image, opts RenderOptions) (*Context, error)
NewContextForImage10(im *Image10) *Context
NewContextForYUV420P10LE(y, u, v []uint16, width, height int, opts YUV420P10LEOptions) (*Context, error)
```

## 10-bit Output

The legacy `NewContext`, `NewContextForImage`, and `NewContextForRGBA` paths keep
their 8-bit RGBA behavior. A zero-value `RenderOptions` also selects the legacy
path for blank canvases. Existing callers therefore see the same pixels,
return values, panics, and log output unless they explicitly opt in or pass a
source image with more than 8 bits.

Use an explicit 10-bit context when the output must remain high depth:

```go
dc, err := gg.NewContextWithOptions(width, height, gg.RenderOptions{
    BitDepth: gg.BitDepth10,
    Dither:   gg.DitherOrdered,
    Logger:   logger, // nil keeps the legacy silent behavior
})
if err != nil {
    return err
}
```

`Image10` stores alpha-premultiplied samples in the range 0..1023. It is the
no-copy input type for direct 10-bit work:

```go
src := gg.NewImage10Rect(width, height)
src.SetRGBA10(x, y, r10, g10, b10, 1023)
dc := gg.NewContextForImage10(src)
if err := dc.SavePNG("out.png"); err != nil {
    return err
}
```

Migration from the existing 8-bit constructor is opt-in:

```go
// Before: legacy 8-bit context.
dc := gg.NewContextForImage(src)

// After: keep 8-bit sources at 8 bits by default, or request 10 bits
// explicitly without changing unrelated call sites.
dc10, err := gg.NewContextForImageWithOptions(src, gg.RenderOptions{
    BitDepth: gg.BitDepth10,
    Dither:   gg.DitherOrdered,
})
```

For a mixed batch, construct one context per item. Depth selection is
per-item; there is no batch-wide minimum:

```go
for _, item := range items {
    dc, err := gg.NewContextForImageWithOptions(item, gg.RenderOptions{
        Dither: gg.DitherOrdered,
    })
    // 8-bit item -> 8-bit context; 10-bit item -> 10-bit context.
}
```

PNG output uses a 16-bit container for the 10-bit samples. JPEG is rejected
without writing a file because it cannot represent the target depth. A raw
`yuv420p10le` frame can be produced explicitly; muxing into MP4 or another
container remains the caller's responsibility:

```go
err := dc.Save("frame.yuv", gg.EncodeOptions{
    Format: gg.FormatYUV420P10LE,
    Matrix: gg.YUVMatrixBT709,
})
```

Bit-depth decisions are logged only when `RenderOptions.Logger` is set. The
stable fields are `level`, `decision`, and either `enter`, `exit`, or `fail`.
For example:

```text
gg: level=debug decision=context_from_image enter format=*image.RGBA source_bit_depth=8 requested_bit_depth=0
gg: level=debug decision=context_from_image exit source_bit_depth=8 target_bit_depth=8 dither=none
gg: level=error decision=encode fail format=jpeg bit_depth=10 error="..."
```

`Context` remains non-thread-safe, matching the existing API. Separate
contexts, including 10-bit contexts, can be rendered concurrently. The ordered
dither table is read-only, so `go test -race` is the verification path.

Current boundaries: target depths are 8 and 10 only; PNG is the supported
high-depth still-image encoder; YUV covers 4:2:0 planar input/output with
BT.709/BT.601 matrices; and container muxing is intentionally out of scope.
Future depth expansion should start in `bitdepth.go` (depth selection),
`pattern10.go` (compositing), and `yuv.go` (pixel-format conversion).

## Drawing Functions

Ever used a graphics library that didn't have functions for drawing rectangles
or circles? What a pain!

```go
DrawPoint(x, y, r float64)
DrawLine(x1, y1, x2, y2 float64)
DrawRectangle(x, y, w, h float64)
DrawRoundedRectangle(x, y, w, h, r float64)
DrawCircle(x, y, r float64)
DrawArc(x, y, r, angle1, angle2 float64)
DrawEllipse(x, y, rx, ry float64)
DrawEllipticalArc(x, y, rx, ry, angle1, angle2 float64)
DrawRegularPolygon(n int, x, y, r, rotation float64)
DrawImage(im image.Image, x, y int)
DrawImageAnchored(im image.Image, x, y int, ax, ay float64)
SetPixel(x, y int)

MoveTo(x, y float64)
LineTo(x, y float64)
QuadraticTo(x1, y1, x2, y2 float64)
CubicTo(x1, y1, x2, y2, x3, y3 float64)
ClosePath()
ClearPath()
NewSubPath()

Clear()
Stroke()
Fill()
StrokePreserve()
FillPreserve()
```

It is often desired to center an image at a point. Use `DrawImageAnchored` with `ax` and `ay` set to 0.5 to do this. Use 0 to left or top align. Use 1 to right or bottom align. `DrawStringAnchored` does the same for text, so you don't need to call `MeasureString` yourself.

## Text Functions

It will even do word wrap for you!

```go
DrawString(s string, x, y float64)
DrawStringAnchored(s string, x, y, ax, ay float64)
DrawStringWrapped(s string, x, y, ax, ay, width, lineSpacing float64, align Align)
MeasureString(s string) (w, h float64)
MeasureMultilineString(s string, lineSpacing float64) (w, h float64)
WordWrap(s string, w float64) []string
SetFontFace(fontFace font.Face)
LoadFontFace(path string, points float64) error
```

## Color Functions

Colors can be set in several different ways for your convenience.

```go
SetRGB(r, g, b float64)
SetRGBA(r, g, b, a float64)
SetRGB255(r, g, b int)
SetRGBA255(r, g, b, a int)
SetColor(c color.Color)
SetHexColor(x string)
```

## Stroke & Fill Options

```go
SetLineWidth(lineWidth float64)
SetLineCap(lineCap LineCap)
SetLineJoin(lineJoin LineJoin)
SetDash(dashes ...float64)
SetDashOffset(offset float64)
SetFillRule(fillRule FillRule)
```

## Gradients & Patterns

`gg` supports linear, radial and conic gradients and surface patterns. You can also implement your own patterns.

```go
SetFillStyle(pattern Pattern)
SetStrokeStyle(pattern Pattern)
NewSolidPattern(color color.Color)
NewLinearGradient(x0, y0, x1, y1 float64)
NewRadialGradient(x0, y0, r0, x1, y1, r1 float64)
NewConicGradient(cx, cy, deg float64)
NewSurfacePattern(im image.Image, op RepeatOp)
```

## Transformation Functions

```go
Identity()
Translate(x, y float64)
Scale(x, y float64)
Rotate(angle float64)
Shear(x, y float64)
ScaleAbout(sx, sy, x, y float64)
RotateAbout(angle, x, y float64)
ShearAbout(sx, sy, x, y float64)
TransformPoint(x, y float64) (tx, ty float64)
InvertY()
```

It is often desired to rotate or scale about a point that is not the origin. The functions `RotateAbout`, `ScaleAbout`, `ShearAbout` are provided as a convenience.

`InvertY` is provided in case Y should increase from bottom to top vs. the default top to bottom.

## Stack Functions

Save and restore the state of the context. These can be nested.

```go
Push()
Pop()
```

## Clipping Functions

Use clipping regions to restrict drawing operations to an area that you
defined using paths.

```go
Clip()
ClipPreserve()
ResetClip()
AsMask() *image.Alpha
SetMask(mask *image.Alpha)
InvertMask()
```

## Helper Functions

Sometimes you just don't want to write these yourself.

```go
Radians(degrees float64) float64
Degrees(radians float64) float64
LoadImage(path string) (image.Image, error)
LoadPNG(path string) (image.Image, error)
SavePNG(path string, im image.Image) error
```

![Separator](http://i.imgur.com/fsUvnPB.png)

## Another Example

See the output of this example below.

```go
package main

import "github.com/fogleman/gg"

func main() {
	const S = 1024
	dc := gg.NewContext(S, S)
	dc.SetRGBA(0, 0, 0, 0.1)
	for i := 0; i < 360; i += 15 {
		dc.Push()
		dc.RotateAbout(gg.Radians(float64(i)), S/2, S/2)
		dc.DrawEllipse(S/2, S/2, S*7/16, S/8)
		dc.Fill()
		dc.Pop()
	}
	dc.SavePNG("out.png")
}
```

![Ellipses](http://i.imgur.com/J9CBZef.png)
