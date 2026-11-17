package gg

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// OutputFormat identifies an explicit output path.
type OutputFormat string

const (
	FormatPNG         OutputFormat = "png"
	FormatJPEG        OutputFormat = "jpeg"
	FormatYUV420P10LE OutputFormat = "yuv420p10le"
	FormatMP4         OutputFormat = "mp4"
)

// EncodeOptions selects an output format and format-specific settings.
type EncodeOptions struct {
	Format  OutputFormat
	Quality int
	Matrix  YUVMatrix
}

// UnsupportedOutputFormatError reports a format or container that cannot be
// produced by this package.
type UnsupportedOutputFormatError struct {
	Format      string
	Suggestions []string
}

func (e *UnsupportedOutputFormatError) Error() string {
	return fmt.Sprintf("gg: unsupported output format %q; use %v", e.Format, e.Suggestions)
}

func newUnsupportedJPEGError(depth int) error {
	return &UnsupportedBitDepthError{
		Format:   "jpeg",
		BitDepth: depth,
		Suggestions: []string{
			"png (16-bit container)",
			"yuv420p10le with an external muxer",
		},
	}
}

// Encode writes the context using an explicit format. Unsupported bit-depth
// combinations return an error before any bytes are written.
func (dc *Context) Encode(w io.Writer, opts EncodeOptions) error {
	dc.logBitDepth("debug", "encode", "enter format=%s bit_depth=%d", opts.Format, dc.BitDepth())
	err := dc.encode(w, opts)
	if err != nil {
		dc.logBitDepth("error", "encode", "fail format=%s bit_depth=%d error=%q", opts.Format, dc.BitDepth(), err)
		return err
	}
	dc.logBitDepth("debug", "encode", "exit format=%s bit_depth=%d", opts.Format, dc.BitDepth())
	return nil
}

func (dc *Context) encode(w io.Writer, opts EncodeOptions) error {
	switch opts.Format {
	case FormatPNG:
		return png.Encode(w, dc.Image())
	case FormatJPEG:
		if dc.BitDepth() != BitDepth8 {
			return newUnsupportedJPEGError(dc.BitDepth())
		}
		quality := opts.Quality
		var jpegOpts *jpeg.Options
		if quality != 0 {
			jpegOpts = &jpeg.Options{Quality: quality}
		}
		return jpeg.Encode(w, dc.Image(), jpegOpts)
	case FormatYUV420P10LE:
		if dc.im10 == nil {
			return &UnsupportedBitDepthError{
				Format:   string(FormatYUV420P10LE),
				BitDepth: dc.BitDepth(),
				Suggestions: []string{
					"an explicit 10-bit context",
					"png (8-bit path)",
				},
			}
		}
		yuv, err := image10ToYUV420P10LE(dc.im10, opts.Matrix)
		if err != nil {
			return err
		}
		_, err = yuv.WriteTo(w)
		return err
	case FormatMP4:
		return &UnsupportedOutputFormatError{
			Format: string(FormatMP4),
			Suggestions: []string{
				"png for image delivery",
				"yuv420p10le plus an external 10-bit-capable muxer",
			},
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutputFormat, opts.Format)
	}
}

// Save writes a complete output atomically, so a failed encode does not leave
// a partial or depth-inconsistent file behind.
func (dc *Context) Save(path string, opts EncodeOptions) error {
	dc.logBitDepth("debug", "save", "enter format=%s bit_depth=%d", opts.Format, dc.BitDepth())
	if opts.Format == FormatJPEG && dc.BitDepth() != BitDepth8 {
		err := newUnsupportedJPEGError(dc.BitDepth())
		dc.logBitDepth("error", "save", "fail format=%s bit_depth=%d error=%q", opts.Format, dc.BitDepth(), err)
		return err
	}
	err := saveAtomic(path, func(w io.Writer) error {
		return dc.encode(w, opts)
	})
	if err != nil {
		dc.logBitDepth("error", "save", "fail format=%s bit_depth=%d error=%q", opts.Format, dc.BitDepth(), err)
		return err
	}
	dc.logBitDepth("debug", "save", "exit format=%s bit_depth=%d", opts.Format, dc.BitDepth())
	return nil
}

func savePNGAtomic(path string, im image.Image) error {
	return saveAtomic(path, func(w io.Writer) error {
		return png.Encode(w, im)
	})
}

func saveAtomic(path string, encode func(io.Writer) error) (err error) {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".gg-*")
	if err != nil {
		return err
	}
	tmpName := file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err = encode(file); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
