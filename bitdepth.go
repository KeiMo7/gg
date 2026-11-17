package gg

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"strings"
)

const (
	// BitDepth8 is the legacy 8-bit RGBA rendering path.
	BitDepth8 = 8
	// BitDepth10 enables the high-depth rendering path.
	BitDepth10 = 10
)

// DitherMode controls how high-precision samples are quantized to 10 bits.
type DitherMode int

const (
	// DitherNone rounds to the nearest 10-bit sample.
	DitherNone DitherMode = iota
	// DitherOrdered applies a deterministic 8x8 Bayer pattern. It is a no-op
	// for samples that are already exactly representable at 10 bits.
	DitherOrdered
)

func (m DitherMode) String() string {
	switch m {
	case DitherNone:
		return "none"
	case DitherOrdered:
		return "ordered"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// RenderOptions controls bit-depth decisions for a Context. The zero value
// preserves the legacy 8-bit behavior for canvas constructors and enables
// automatic 10-bit rendering when the source image carries more than 8 bits.
type RenderOptions struct {
	// BitDepth may be 0 (automatic), 8, or 10.
	BitDepth int
	// SourceBitDepth optionally declares the source sample precision. When it
	// is zero, the precision is inferred from the image type.
	SourceBitDepth int
	// Dither controls quantization when high-precision samples are written to
	// a 10-bit target.
	Dither DitherMode
	// Logger receives bit-depth decisions. A nil logger preserves the legacy
	// silent behavior.
	Logger *log.Logger
}

// UnsupportedBitDepthError is returned when an output path cannot represent
// the context's bit depth. Callers must opt into a supported format rather
// than receiving a silent 8-bit downgrade.
type UnsupportedBitDepthError struct {
	Format      string
	BitDepth    int
	Suggestions []string
}

func (e *UnsupportedBitDepthError) Error() string {
	var suggestion string
	if len(e.Suggestions) > 0 {
		suggestion = "; use " + strings.Join(e.Suggestions, " or ")
	}
	return fmt.Sprintf("gg: %s does not support %d-bit output%s", e.Format, e.BitDepth, suggestion)
}

// BitDepthMismatchError reports a source whose declared precision does not
// match its samples.
type BitDepthMismatchError struct {
	Declared int
	Observed int
}

func (e *BitDepthMismatchError) Error() string {
	return fmt.Sprintf("gg: source declared %d-bit but contains %d-bit samples", e.Declared, e.Observed)
}

var (
	// ErrUnsupportedOutputFormat is returned by Encode for unknown formats.
	ErrUnsupportedOutputFormat = errors.New("gg: unsupported output format")
)

func validateDitherMode(mode DitherMode) error {
	switch mode {
	case DitherNone, DitherOrdered:
		return nil
	default:
		return fmt.Errorf("gg: unsupported dither mode %d", int(mode))
	}
}

func validateTargetBitDepth(depth int) error {
	switch depth {
	case 0, BitDepth8, BitDepth10:
		return nil
	default:
		return fmt.Errorf("gg: unsupported bit depth %d: use 8 or 10", depth)
	}
}

func effectiveTargetBitDepth(sourceDepth, requested int) int {
	if requested != 0 {
		return requested
	}
	if sourceDepth > BitDepth8 {
		return BitDepth10
	}
	return BitDepth8
}

func sourceBitDepthOf(im image.Image) int {
	if im == nil {
		return BitDepth8
	}
	if declared, ok := im.(interface{ BitDepth() int }); ok {
		return declared.BitDepth()
	}
	switch im.(type) {
	case *image.RGBA64, *image.NRGBA64, *image.Gray16, *image.Alpha16:
		return 16
	}
	switch im.ColorModel() {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model:
		return 16
	default:
		return BitDepth8
	}
}

func sourceFormatOf(im image.Image) string {
	if im == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", im)
}

// ValidateBitDepth verifies that an image's samples fit its declared bit
// depth. The main 10-bit mismatch case is a source declared as 8-bit whose
// samples cannot be represented by the 8-to-16-bit replication rule.
func ValidateBitDepth(im image.Image, declared int) error {
	if im == nil {
		return errors.New("gg: cannot validate a nil image")
	}
	if declared < BitDepth8 || declared > 16 {
		return fmt.Errorf("gg: unsupported declared bit depth %d: use 8 through 16", declared)
	}
	if declared > BitDepth8 {
		return nil
	}
	if isKnown8BitImage(im) {
		return nil
	}
	bounds := im.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := im.At(x, y).RGBA()
			if !fits8BitSample(r) || !fits8BitSample(g) || !fits8BitSample(b) || !fits8BitSample(a) {
				return &BitDepthMismatchError{Declared: declared, Observed: 16}
			}
		}
	}
	return nil
}

func isKnown8BitImage(im image.Image) bool {
	switch im.(type) {
	case *image.RGBA, *image.NRGBA, *image.Gray, *image.Alpha,
		*image.Paletted, *image.YCbCr, *image.CMYK:
		return true
	default:
		return false
	}
}

func fits8BitSample(v uint32) bool {
	return uint8(v>>8) == uint8(v)
}

func logBitDepthDecision(logger *log.Logger, level, decision, format string, args ...interface{}) {
	if logger == nil {
		return
	}
	logger.Printf("gg: level=%s decision=%s %s", level, decision, fmt.Sprintf(format, args...))
}

func (dc *Context) SetLogger(logger *log.Logger) {
	dc.logger = logger
}

func (dc *Context) logBitDepth(level, decision, format string, args ...interface{}) {
	logBitDepthDecision(dc.logger, level, decision, format, args...)
}
