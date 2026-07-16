package storage

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

// makeImage builds a solid-color image of the given size.
func makeImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 200, B: 40, A: 255})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestSniffContentType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", encodeJPEG(t, makeImage(4, 4)), ContentTypeJPEG},
		{"png", encodePNG(t, makeImage(4, 4)), ContentTypePNG},
		{"pdf", []byte("%PDF-1.7\n..."), ContentTypePDF},
		{"plain text", []byte("hello world, not an image"), ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SniffContentType(tt.data); got != tt.want {
				t.Errorf("SniffContentType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeJPEG(t *testing.T) {
	in := encodeJPEG(t, makeImage(16, 16))
	out, ct, err := Normalize(in, "image/jpeg")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if ct != ContentTypeJPEG {
		t.Errorf("content type = %q, want %q", ct, ContentTypeJPEG)
	}
	if SniffContentType(out) != ContentTypeJPEG {
		t.Errorf("output is not a valid JPEG")
	}
}

func TestNormalizePNGStaysPNG(t *testing.T) {
	in := encodePNG(t, makeImage(16, 16))
	out, ct, err := Normalize(in, "image/png")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if ct != ContentTypePNG {
		t.Errorf("content type = %q, want %q", ct, ContentTypePNG)
	}
	if SniffContentType(out) != ContentTypePNG {
		t.Errorf("output is not a valid PNG")
	}
}

func TestNormalizeStripsMetadata(t *testing.T) {
	// A JPEG carrying a real APP1/EXIF segment should come back without it after
	// the decode + re-encode round trip. Build a structurally valid segment so
	// the decoder skips it rather than choking.
	in := encodeJPEG(t, makeImage(16, 16))
	marker := []byte("Exif\x00\x00")
	if bytes.Contains(in, marker) {
		t.Skip("stdlib jpeg already emits an Exif marker; skipping")
	}
	// APP1 segment: FF E1, 2-byte length (covers length bytes + payload), payload.
	payload := append(append([]byte{}, marker...), []byte("GPS 1.234,5.678")...)
	segLen := len(payload) + 2
	app1 := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen & 0xFF)}
	app1 = append(app1, payload...)

	tampered := append([]byte{}, in[:2]...) // SOI (FF D8)
	tampered = append(tampered, app1...)    // inject EXIF right after SOI
	tampered = append(tampered, in[2:]...)  // rest of the original JPEG

	out, _, err := Normalize(tampered, "image/jpeg")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if bytes.Contains(out, marker) {
		t.Errorf("EXIF metadata marker survived normalization")
	}
}

func TestNormalizePDFPassthrough(t *testing.T) {
	in := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF")
	out, ct, err := Normalize(in, "application/pdf")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if ct != ContentTypePDF {
		t.Errorf("content type = %q, want %q", ct, ContentTypePDF)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("PDF bytes were altered")
	}
}

func TestNormalizeRejectsUnsupported(t *testing.T) {
	_, _, err := Normalize([]byte("just some random text payload here"), "text/plain")
	if !errors.Is(err, ErrUnsupportedMediaType) {
		t.Errorf("err = %v, want ErrUnsupportedMediaType", err)
	}
}

func TestNormalizeRejectsFakePDF(t *testing.T) {
	// Claims to be a PDF by declared type but has no magic bytes.
	_, _, err := Normalize([]byte("NOTAPDF but long enough to sniff"), "application/pdf")
	if !errors.Is(err, ErrUnsupportedMediaType) {
		t.Errorf("err = %v, want ErrUnsupportedMediaType", err)
	}
}

func TestNormalizeRejectsDecompressionBomb(t *testing.T) {
	// A tiny PNG that decodes to more than MaxImagePixels. A solid-color image
	// compresses tightly, so this stays a small upload but blows the pixel cap.
	side := 8000 // 64 MP > 50 MP cap
	bomb := encodePNG(t, makeImage(side, side))
	_, _, err := Normalize(bomb, "image/png")
	if !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("err = %v, want ErrImageTooLarge", err)
	}
}

func TestNormalizeReaderRoundTrip(t *testing.T) {
	in := encodeJPEG(t, makeImage(8, 8))
	r, ct, err := NormalizeReader(bytes.NewReader(in), "image/jpeg")
	if err != nil {
		t.Fatalf("NormalizeReader: %v", err)
	}
	if ct != ContentTypeJPEG {
		t.Errorf("content type = %q, want %q", ct, ContentTypeJPEG)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if SniffContentType(out) != ContentTypeJPEG {
		t.Errorf("reader output is not a valid JPEG")
	}
}
