package storage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	// Register decoders for the raster formats we accept. GIF is registered so
	// that a mislabelled GIF is decoded and rejected rather than silently
	// stored, but it is not part of the allowlist.
	_ "image/gif"

	"golang.org/x/image/webp"
)

// Content types on the receipt attachment allowlist. Anything else is rejected
// before it reaches storage. HEIC is intentionally absent (see plan 017, R1):
// there is no reliable pure-Go decoder and browsers cannot render it in <img>.
const (
	ContentTypeJPEG = "image/jpeg"
	ContentTypePNG  = "image/png"
	ContentTypeWebP = "image/webp"
	ContentTypePDF  = "application/pdf"
)

// MaxImagePixels bounds the decoded dimensions of a raster image (50 MP) to
// defuse decompression bombs: a small compressed file that expands to an
// enormous bitmap and exhausts memory during decode.
const MaxImagePixels = 50 * 1000 * 1000

// jpegQuality is the re-encode quality for normalized raster output. High
// enough that receipts stay legible, low enough to keep file sizes sane.
const jpegQuality = 85

var (
	// ErrUnsupportedMediaType is returned when the sniffed content type is not
	// on the allowlist.
	ErrUnsupportedMediaType = errors.New("storage: unsupported media type")
	// ErrImageTooLarge is returned when a raster image's decoded dimensions
	// exceed MaxImagePixels (decompression-bomb defense).
	ErrImageTooLarge = errors.New("storage: image dimensions exceed limit")
	// ErrCorruptImage is returned when an image cannot be decoded.
	ErrCorruptImage = errors.New("storage: image could not be decoded")
)

// SniffContentType inspects the leading bytes of an upload and returns the
// canonical allowlisted content type, or "" if the bytes are not a supported
// format. It never trusts a client-declared Content-Type or file extension.
func SniffContentType(head []byte) string {
	// http.DetectContentType returns e.g. "image/jpeg; charset=..." only for
	// text; the binary types we care about come back bare, but strip params to
	// be safe.
	detected := http.DetectContentType(head)
	switch detected {
	case ContentTypeJPEG, "image/jpg":
		return ContentTypeJPEG
	case ContentTypePNG:
		return ContentTypePNG
	case ContentTypeWebP:
		return ContentTypeWebP
	case ContentTypePDF:
		return ContentTypePDF
	default:
		return ""
	}
}

// Normalize validates and sanitizes an uploaded receipt. It sniffs the true
// content type from the bytes (ignoring declaredType), rejects anything off the
// allowlist, and for raster images decodes then re-encodes the pixels. The
// re-encode strips all metadata (EXIF/GPS) and, combined with the pixel bound,
// defuses decompression bombs. WebP is transcoded to JPEG because x/image ships
// no WebP encoder and browsers render JPEG universally. PDFs are passed through
// unchanged after a magic-byte check.
//
// It returns the sanitized bytes and their final content type.
func Normalize(data []byte, declaredType string) ([]byte, string, error) {
	_ = declaredType // never trusted; kept for signature clarity and future logging

	sniffed := SniffContentType(data)
	switch sniffed {
	case ContentTypeJPEG, ContentTypePNG, ContentTypeWebP:
		return normalizeRaster(data, sniffed)
	case ContentTypePDF:
		return normalizePDF(data)
	default:
		return nil, "", ErrUnsupportedMediaType
	}
}

// normalizeRaster decodes a raster image, enforces the pixel bound, and
// re-encodes it (dropping metadata). JPEG/PNG keep their format; WebP is
// transcoded to JPEG.
func normalizeRaster(data []byte, sniffed string) ([]byte, string, error) {
	// Bound dimensions before a full decode using the cheap header-only path.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrCorruptImage, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels {
		return nil, "", ErrImageTooLarge
	}

	img, err := decodeRaster(data, sniffed)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrCorruptImage, err)
	}

	var buf bytes.Buffer
	switch sniffed {
	case ContentTypePNG:
		if err := (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(&buf, img); err != nil {
			return nil, "", fmt.Errorf("re-encode png: %w", err)
		}
		return buf.Bytes(), ContentTypePNG, nil
	default: // JPEG and WebP both emit JPEG
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, "", fmt.Errorf("re-encode jpeg: %w", err)
		}
		return buf.Bytes(), ContentTypeJPEG, nil
	}
}

// decodeRaster decodes into an image.Image using the decoder matching the
// sniffed type. WebP needs x/image explicitly; JPEG/PNG go through the stdlib
// registry.
func decodeRaster(data []byte, sniffed string) (image.Image, error) {
	if sniffed == ContentTypeWebP {
		return webp.Decode(bytes.NewReader(data))
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// normalizePDF validates the PDF magic and returns the bytes unchanged.
// Metadata scrubbing of PDFs is deferred (see plan 017).
func normalizePDF(data []byte) ([]byte, string, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, "", ErrUnsupportedMediaType
	}
	return data, ContentTypePDF, nil
}

// NormalizeReader is the io.Reader convenience wrapper around Normalize. It
// reads all of r (callers must apply their own size cap upstream, e.g. via
// http.MaxBytesReader) before sniffing.
func NormalizeReader(r io.Reader, declaredType string) (io.Reader, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	out, ct, err := Normalize(data, declaredType)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(out), ct, nil
}
