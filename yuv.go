package gg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"math"
)

// YUVMatrix identifies the explicit YUV-to-RGB conversion matrix.
type YUVMatrix int

const (
	// YUVMatrixBT709 is the default matrix for HD/10-bit video.
	YUVMatrixBT709 YUVMatrix = iota
	// YUVMatrixBT601 is the standard-definition matrix.
	YUVMatrixBT601
)

func (m YUVMatrix) String() string {
	switch m {
	case YUVMatrixBT709:
		return "bt709"
	case YUVMatrixBT601:
		return "bt601"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// YUV420P10LEOptions controls explicit planar 10-bit YUV conversion.
type YUV420P10LEOptions struct {
	Matrix YUVMatrix
	Logger *log.Logger
}

// YUV420P10LE is a planar 10-bit YUV 4:2:0 frame. Strides are measured in
// uint16 samples.
type YUV420P10LE struct {
	Y, U, V                   []uint16
	YStride, UStride, VStride int
	Rect                      image.Rectangle
}

// NewImage10FromYUV420P10LE converts tightly packed yuv420p10le planes using
// an explicit matrix. Samples outside [0, 1023] are rejected.
func NewImage10FromYUV420P10LE(y, u, v []uint16, width, height int, opts YUV420P10LEOptions) (*Image10, error) {
	logBitDepthDecision(opts.Logger, "debug", "yuv420p10le",
		"enter width=%d height=%d matrix=%s", width, height, opts.Matrix)
	if width <= 0 || height <= 0 {
		err := fmt.Errorf("gg: yuv420p10le dimensions must be positive: %dx%d", width, height)
		logBitDepthDecision(opts.Logger, "error", "yuv420p10le", "fail error=%q", err)
		return nil, err
	}
	if opts.Matrix != YUVMatrixBT709 && opts.Matrix != YUVMatrixBT601 {
		err := fmt.Errorf("gg: unsupported YUV matrix %d", int(opts.Matrix))
		logBitDepthDecision(opts.Logger, "error", "yuv420p10le", "fail error=%q", err)
		return nil, err
	}
	cw, ch := (width+1)/2, (height+1)/2
	if len(y) != width*height || len(u) != cw*ch || len(v) != cw*ch {
		err := fmt.Errorf("gg: yuv420p10le plane size mismatch: y=%d u=%d v=%d want y=%d u=%d v=%d",
			len(y), len(u), len(v), width*height, cw*ch, cw*ch)
		logBitDepthDecision(opts.Logger, "error", "yuv420p10le", "fail error=%q", err)
		return nil, err
	}
	if err := validate10Samples("y", y); err != nil {
		logBitDepthDecision(opts.Logger, "error", "yuv420p10le", "fail error=%q", err)
		return nil, err
	}
	if err := validate10Samples("u", u); err != nil {
		logBitDepthDecision(opts.Logger, "error", "yuv420p10le", "fail error=%q", err)
		return nil, err
	}
	if err := validate10Samples("v", v); err != nil {
		logBitDepthDecision(opts.Logger, "error", "yuv420p10le", "fail error=%q", err)
		return nil, err
	}

	dst := NewImage10Rect(width, height)
	for py := 0; py < height; py++ {
		yRow := py * width
		cRow := (py / 2) * cw
		for px := 0; px < width; px++ {
			yy := float64(y[yRow+px])
			uSample := u[cRow+px/2]
			vSample := v[cRow+px/2]
			r, g, b := yuvToRGB10(yy, uSample, vSample, opts.Matrix)
			dst.SetRGBA10(px, py, r, g, b, 1023)
		}
	}
	logBitDepthDecision(opts.Logger, "debug", "yuv420p10le",
		"exit target_bit_depth=%d matrix=%s", BitDepth10, opts.Matrix)
	return dst, nil
}

// NewContextForYUV420P10LE creates a 10-bit context through the explicit
// yuv420p10le conversion path.
func NewContextForYUV420P10LE(y, u, v []uint16, width, height int, opts YUV420P10LEOptions) (*Context, error) {
	im, err := NewImage10FromYUV420P10LE(y, u, v, width, height, opts)
	if err != nil {
		return nil, err
	}
	dc := NewContextForImage10(im)
	dc.logger = opts.Logger
	dc.sourceFormat = "yuv420p10le"
	dc.logBitDepth("debug", "context_from_image",
		"exit source_format=yuv420p10le source_bit_depth=10 target_bit_depth=10")
	return dc, nil
}

func validate10Samples(plane string, samples []uint16) error {
	for i, sample := range samples {
		if sample > 1023 {
			return fmt.Errorf("%w: %s[%d]=%d exceeds 1023",
				&BitDepthMismatchError{Declared: BitDepth10, Observed: 16}, plane, i, sample)
		}
	}
	return nil
}

func yuvToRGB10(y float64, u, v uint16, matrix YUVMatrix) (uint16, uint16, uint16) {
	cb := float64(u) - 512
	cr := float64(v) - 512
	luma := 1.164 * (y - 64)
	var r, g, b float64
	switch matrix {
	case YUVMatrixBT601:
		r = luma + 1.596*cr
		g = luma - 0.391*cb - 0.813*cr
		b = luma + 2.018*cb
	default:
		r = luma + 1.793*cr
		g = luma - 0.213*cb - 0.533*cr
		b = luma + 2.112*cb
	}
	return clamp10Float(r), clamp10Float(g), clamp10Float(b)
}

func clamp10Float(v float64) uint16 {
	if v <= 0 {
		return 0
	}
	if v >= 1023 {
		return 1023
	}
	return uint16(math.Round(v))
}

func image10ToYUV420P10LE(im *Image10, matrix YUVMatrix) (*YUV420P10LE, error) {
	if im == nil {
		return nil, errors.New("gg: cannot encode a nil 10-bit image")
	}
	if !im.Opaque() {
		return nil, errors.New("gg: yuv420p10le has no alpha channel; composite transparency first")
	}
	if matrix != YUVMatrixBT709 && matrix != YUVMatrixBT601 {
		return nil, fmt.Errorf("gg: unsupported YUV matrix %d", int(matrix))
	}
	w, h := im.Rect.Dx(), im.Rect.Dy()
	cw, ch := (w+1)/2, (h+1)/2
	out := &YUV420P10LE{
		Y:       make([]uint16, w*h),
		U:       make([]uint16, cw*ch),
		V:       make([]uint16, cw*ch),
		YStride: w,
		UStride: cw,
		VStride: cw,
		Rect:    image.Rect(0, 0, w, h),
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := im.RGBA10At(im.Rect.Min.X+x, im.Rect.Min.Y+y)
			yy, cb, cr := rgb10ToYUV(r, g, b, matrix)
			out.Y[y*w+x] = yy
			if x%2 == 0 && y%2 == 0 {
				out.U[(y/2)*cw+x/2] = cb
				out.V[(y/2)*cw+x/2] = cr
			}
		}
	}
	return out, nil
}

func rgb10ToYUV(r, g, b uint16, matrix YUVMatrix) (uint16, uint16, uint16) {
	rf, gf, bf := float64(r)/1023, float64(g)/1023, float64(b)/1023
	var luma, pb, pr float64
	switch matrix {
	case YUVMatrixBT601:
		luma = 0.299*rf + 0.587*gf + 0.114*bf
		pb = (bf - luma) / 1.772
		pr = (rf - luma) / 1.402
	default:
		luma = 0.2126*rf + 0.7152*gf + 0.0722*bf
		pb = (bf - luma) / 1.8556
		pr = (rf - luma) / 1.5748
	}
	return clamp10Float(64 + 876*luma),
		clamp10Float(512 + 896*pb),
		clamp10Float(512 + 896*pr)
}

// WriteTo writes tightly packed y, u, and v planes as little-endian uint16
// samples. The caller is responsible for muxing these planes into a container.
func (p *YUV420P10LE) WriteTo(w io.Writer) (int64, error) {
	if p == nil {
		return 0, errors.New("gg: cannot write a nil yuv420p10le frame")
	}
	wantY := p.Rect.Dx() * p.Rect.Dy()
	cw, ch := (p.Rect.Dx()+1)/2, (p.Rect.Dy()+1)/2
	wantC := cw * ch
	if len(p.Y) < wantY || len(p.U) < wantC || len(p.V) < wantC {
		return 0, errors.New("gg: yuv420p10le plane size mismatch")
	}
	buf := make([]byte, (wantY+2*wantC)*2)
	offset := 0
	for _, plane := range [][]uint16{p.Y[:wantY], p.U[:wantC], p.V[:wantC]} {
		for _, sample := range plane {
			if sample > 1023 {
				return 0, &BitDepthMismatchError{Declared: BitDepth10, Observed: 16}
			}
			binary.LittleEndian.PutUint16(buf[offset:], sample)
			offset += 2
		}
	}
	n, err := w.Write(buf)
	if err != nil {
		return int64(n), err
	}
	if n != len(buf) {
		return int64(n), io.ErrShortWrite
	}
	return int64(n), nil
}
