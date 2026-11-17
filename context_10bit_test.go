package gg

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDefaultOptionsPreserveRGBA(t *testing.T) {
	drawComparableScene := func(dc *Context) {
		dc.SetRGB(1, 1, 1)
		dc.Clear()
		dc.SetRGBA(0.2, 0.4, 0.6, 0.75)
		dc.DrawCircle(16, 16, 12)
		dc.Fill()
		dc.SetRGB(0.1, 0.8, 0.3)
		dc.SetLineWidth(2)
		dc.DrawLine(0, 0, 32, 32)
		dc.Stroke()
	}

	legacy := NewContext(32, 32)
	withDefaults, err := NewContextWithOptions(32, 32, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	drawComparableScene(legacy)
	drawComparableScene(withDefaults)

	if legacy.BitDepth() != BitDepth8 || withDefaults.BitDepth() != BitDepth8 {
		t.Fatalf("default bit depths = %d, %d", legacy.BitDepth(), withDefaults.BitDepth())
	}
	if _, ok := withDefaults.Image().(*image.RGBA); !ok {
		t.Fatalf("default image type = %T, want *image.RGBA", withDefaults.Image())
	}
	if !bytes.Equal(legacy.im.Pix, withDefaults.im.Pix) {
		t.Fatal("zero-value options changed the legacy 8-bit pixels")
	}
}

func Test10BitPassthrough(t *testing.T) {
	src := NewImage10Rect(4, 2)
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			value := uint16(x*257 + y*17)
			src.SetRGBA10(x, y, value, 1023-value, value/2, 1023)
		}
	}
	dc := NewContextForImage10(src)
	if dc.BitDepth() != BitDepth10 {
		t.Fatalf("bit depth = %d, want 10", dc.BitDepth())
	}
	if dc.SourceBitDepth() != BitDepth10 {
		t.Fatalf("source bit depth = %d, want 10", dc.SourceBitDepth())
	}
	if dc.Image10() != src {
		t.Fatal("NewContextForImage10 unexpectedly copied the image")
	}

	var encoded bytes.Buffer
	if err := dc.EncodePNG(&encoded); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ColorModel() != color.RGBA64Model {
		t.Fatalf("encoded color model = %T, want color.RGBA64Model", decoded.ColorModel())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			wr, wg, wb, wa := src.At(x, y).RGBA()
			gr, gg, gb, ga := decoded.At(x, y).RGBA()
			if gr != wr || gg != wg || gb != wb || ga != wa {
				t.Fatalf("pixel (%d,%d) rgba = %d,%d,%d,%d, want %d,%d,%d,%d",
					x, y, gr, gg, gb, ga, wr, wg, wb, wa)
			}
		}
	}
}

func Test8BitTo10BitUpscaleHasNoDitherNoise(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 256, 1))
	for x := 0; x < 256; x++ {
		src.SetRGBA(x, 0, color.RGBA{uint8(x), uint8(255 - x), uint8(x / 2), 255})
	}
	for _, mode := range []DitherMode{DitherNone, DitherOrdered} {
		dc, err := NewContextForImageWithOptions(src, RenderOptions{
			BitDepth: BitDepth10,
			Dither:   mode,
		})
		if err != nil {
			t.Fatal(err)
		}
		if dc.BitDepth() != BitDepth10 || dc.SourceBitDepth() != BitDepth8 {
			t.Fatalf("depths = %d/%d, want target 10/source 8", dc.BitDepth(), dc.SourceBitDepth())
		}
		for x := 0; x < 256; x++ {
			r, g, b, a := dc.Image10().RGBA10At(x, 0)
			wantR := replicate8To10(uint8(x))
			wantG := replicate8To10(uint8(255 - x))
			wantB := replicate8To10(uint8(x / 2))
			if r != wantR || g != wantG || b != wantB || a != 1023 {
				t.Fatalf("dither=%s x=%d rgba10 = %d,%d,%d,%d", mode, x, r, g, b, a)
			}
		}
	}
}

func replicate8To10(v uint8) uint16 {
	x := uint16(v)
	return x<<2 | x>>6
}

func TestMixedBitDepthBatchIsDecidedPerItem(t *testing.T) {
	im8 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			im8.SetRGBA(x, y, color.RGBA{uint8(40 + x), uint8(80 + y), 120, 255})
		}
	}
	im10 := NewImage10Rect(2, 2)
	im10.SetRGBA10(0, 0, 1000, 20, 30, 1023)
	im10.SetRGBA10(1, 0, 900, 40, 50, 1023)
	im10.SetRGBA10(0, 1, 800, 60, 70, 1023)
	im10.SetRGBA10(1, 1, 700, 80, 90, 1023)
	im16 := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	im16.SetNRGBA64(0, 0, color.NRGBA64{R: 0xc000, G: 0x8000, B: 0x4000, A: 0xffff})

	inputs := []image.Image{im8, im10, im8, im10, im16}
	want := []int{BitDepth8, BitDepth10, BitDepth8, BitDepth10, BitDepth10}
	for i, input := range inputs {
		dc, err := NewContextForImageWithOptions(input, RenderOptions{})
		if err != nil {
			t.Fatalf("item %d: %v", i, err)
		}
		if dc.BitDepth() != want[i] {
			t.Fatalf("item %d bit depth = %d, want %d", i, dc.BitDepth(), want[i])
		}
	}
	if dc := NewContextForImage(im16); dc.BitDepth() != BitDepth10 {
		t.Fatalf("NewContextForImage(16-bit) bit depth = %d, want 10", dc.BitDepth())
	}
}

func Test10BitDrawImageKeepsHighSamples(t *testing.T) {
	src := NewImage10Rect(1, 1)
	src.SetRGBA10(0, 0, 800, 400, 200, 1023)
	dc := NewContextForImage10(NewImage10Rect(2, 2))
	dc.DrawImage(src, 0, 0)
	r, g, b, a := dc.Image10().RGBA10At(0, 0)
	if r != 800 || g != 400 || b != 200 || a != 1023 {
		t.Fatalf("drawn rgba10 = %d,%d,%d,%d", r, g, b, a)
	}
}

func Test10BitClipUsesHighDepthMask(t *testing.T) {
	dc, err := NewContextWithOptions(8, 8, RenderOptions{BitDepth: BitDepth10})
	if err != nil {
		t.Fatal(err)
	}
	dc.DrawRectangle(0, 0, 4, 8)
	dc.Clip()
	dc.SetRGB10(1023, 0, 0)
	dc.DrawRectangle(0, 0, 8, 8)
	dc.Fill()

	if r, _, _, a := dc.Image10().RGBA10At(1, 1); r != 1023 || a != 1023 {
		t.Fatalf("inside clip rgba10 = %d,%d", r, a)
	}
	if _, _, _, a := dc.Image10().RGBA10At(6, 1); a != 0 {
		t.Fatalf("outside clip alpha = %d, want 0", a)
	}
}

func Test10BitGradientHasNoBanding(t *testing.T) {
	const width = 1024
	dc, err := NewContextWithOptions(width, 8, RenderOptions{BitDepth: BitDepth10})
	if err != nil {
		t.Fatal(err)
	}
	g := NewLinearGradient(0, 0, width-1, 0)
	g.AddColorStop(0, color.RGBA{0, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{255, 255, 255, 255})
	dc.SetFillStyle(g)
	dc.DrawRectangle(0, 0, width, 8)
	dc.Fill()

	seen := make(map[uint16]struct{}, width)
	previous := uint16(0)
	maxDelta := 0
	for x := 0; x < width; x++ {
		r, _, _, _ := dc.Image10().RGBA10At(x, 4)
		seen[r] = struct{}{}
		if x > 0 {
			delta := int(r) - int(previous)
			if delta < 0 {
				delta = -delta
			}
			if delta > maxDelta {
				maxDelta = delta
			}
		}
		previous = r
	}
	t.Logf("distinct=%d maxDelta=%d", len(seen), maxDelta)
	if len(seen) < 990 {
		t.Fatalf("gradient has %d distinct 10-bit levels, want at least 990", len(seen))
	}
	if maxDelta > 2 {
		t.Fatalf("adjacent 10-bit gradient delta = %d, want <= 2", maxDelta)
	}
}

func TestBitDepthMismatchDetectedAndLogged(t *testing.T) {
	bad := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	bad.SetNRGBA64(0, 0, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
	if err := ValidateBitDepth(bad, BitDepth8); err == nil {
		t.Fatal("expected a bit-depth mismatch")
	} else if _, ok := err.(*BitDepthMismatchError); !ok {
		t.Fatalf("mismatch error type = %T", err)
	}

	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	if _, err := NewContextForImageWithOptions(bad, RenderOptions{
		SourceBitDepth: BitDepth8,
		Logger:         logger,
	}); err == nil {
		t.Fatal("expected constructor to reject the mismatch")
	}
	if !strings.Contains(logs.String(), "level=debug") ||
		!strings.Contains(logs.String(), "level=error") ||
		!strings.Contains(logs.String(), "fail") {
		t.Fatalf("missing enter/fail logs:\n%s", logs.String())
	}

	logs.Reset()
	good := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if _, err := NewContextForImageWithOptions(good, RenderOptions{
		BitDepth: BitDepth10,
		Logger:   logger,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "exit source_bit_depth=8 target_bit_depth=10") {
		t.Fatalf("missing exit log:\n%s", logs.String())
	}
}

func TestUnsupported10BitEncoderDoesNotLeaveFile(t *testing.T) {
	dc := NewContextForImage10(NewImage10Rect(2, 2))
	path := filepath.Join(t.TempDir(), "out.jpg")
	err := dc.SaveJPG(path, 90)
	if err == nil {
		t.Fatal("expected JPEG to reject 10-bit output")
	}
	if _, ok := err.(*UnsupportedBitDepthError); !ok {
		t.Fatalf("error type = %T, want *UnsupportedBitDepthError", err)
	}
	if !strings.Contains(err.Error(), "png") {
		t.Fatalf("error does not contain a replacement suggestion: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("failed encode left a file behind: %v", statErr)
	}

	err = dc.Encode(bytes.NewBuffer(nil), EncodeOptions{Format: FormatMP4})
	if err == nil || !strings.Contains(err.Error(), "yuv420p10le") {
		t.Fatalf("container error = %v", err)
	}
	path = filepath.Join(t.TempDir(), "out.mp4")
	if err := dc.Save(path, EncodeOptions{Format: FormatMP4}); err == nil {
		t.Fatal("expected MP4 save to fail")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("failed container save left a file behind: %v", statErr)
	}
}

func TestYUV420P10LEExplicitConversion(t *testing.T) {
	y := []uint16{64, 128, 512, 940}
	u := []uint16{512}
	v := []uint16{512}
	im, err := NewImage10FromYUV420P10LE(y, u, v, 2, 2, YUV420P10LEOptions{Matrix: YUVMatrixBT709})
	if err != nil {
		t.Fatal(err)
	}
	if im.BitDepth() != BitDepth10 {
		t.Fatalf("bit depth = %d, want 10", im.BitDepth())
	}
	if _, _, _, a := im.RGBA10At(0, 0); a != 1023 {
		t.Fatalf("alpha = %d, want 1023", a)
	}

	var out bytes.Buffer
	frame, err := image10ToYUV420P10LE(im, YUVMatrixBT709)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := frame.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	wantLen := (len(y) + len(u) + len(v)) * 2
	if out.Len() != wantLen {
		t.Fatalf("encoded bytes = %d, want %d", out.Len(), wantLen)
	}
	if got := binary.LittleEndian.Uint16(out.Bytes()[:2]); got != frame.Y[0] {
		t.Fatalf("first Y sample = %d, want %d", got, frame.Y[0])
	}

	if _, err := NewImage10FromYUV420P10LE(
		[]uint16{0, 1, 2, 1024}, []uint16{512}, []uint16{512}, 2, 2,
		YUV420P10LEOptions{},
	); err == nil {
		t.Fatal("expected out-of-range 10-bit YUV sample to fail")
	}
}

func Test10BitPNGSizeStaysReasonable(t *testing.T) {
	render := func(depth int) []byte {
		dc, err := NewContextWithOptions(512, 512, RenderOptions{BitDepth: depth})
		if err != nil {
			t.Fatal(err)
		}
		g := NewLinearGradient(0, 0, 511, 0)
		g.AddColorStop(0, color.RGBA{16, 32, 64, 255})
		g.AddColorStop(1, color.RGBA{224, 192, 160, 255})
		dc.SetFillStyle(g)
		dc.DrawRectangle(0, 0, 512, 512)
		dc.Fill()
		var out bytes.Buffer
		if err := dc.EncodePNG(&out); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	png8 := render(BitDepth8)
	png10 := render(BitDepth10)
	ratio := float64(len(png10)) / float64(len(png8))
	t.Logf("png8=%d png10=%d ratio=%.2f", len(png8), len(png10), ratio)
	if ratio > 2.2 {
		t.Fatalf("10-bit PNG is %.2fx the 8-bit size", ratio)
	}
}

func TestConcurrentMixedDepthRendering(t *testing.T) {
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			depth := BitDepth8
			if i%2 == 1 {
				depth = BitDepth10
			}
			dc, err := NewContextWithOptions(64, 64, RenderOptions{BitDepth: depth})
			if err != nil {
				errs <- err
				return
			}
			dc.SetRGB10(uint16(100+i), 300, 700)
			dc.DrawCircle(32, 32, 20)
			dc.Fill()
			if dc.BitDepth() != depth {
				errs <- &BitDepthMismatchError{Declared: depth, Observed: dc.BitDepth()}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
