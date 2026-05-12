package floorplan

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for DecodeConfig
	_ "image/png"  // register PNG decoder for DecodeConfig
	"io"
	"net/http"
)

const (
	// MaxUploadBytes is the maximum accepted multipart body size (D-19).
	// http.MaxBytesReader caps the request at this limit before any parsing.
	MaxUploadBytes = 10 << 20 // 10 MiB

	// MaxDimension is the maximum pixel dimension in either axis (D-19).
	// Checked via image.DecodeConfig (header-only) BEFORE writing to disk
	// to prevent disk-exhaustion via oversized image uploads (T-05-05-01).
	MaxDimension = 8192
)

var (
	// ErrUnsupportedMIME is returned by ValidateImageHeader when the sniffed
	// MIME type is not image/png or image/jpeg. PDF is explicitly called out
	// in the error message (D-17 — server never processes PDF).
	ErrUnsupportedMIME = errors.New("unsupported_mime_type")

	// ErrDimensionTooLarge is returned by ProbeDimensions when either axis
	// exceeds MaxDimension (8192 px).
	ErrDimensionTooLarge = errors.New("dimensions_too_large")

	// ErrImageDecodeFailed is returned by ProbeDimensions when image.DecodeConfig
	// cannot parse the image header (corrupted or unrecognised format).
	ErrImageDecodeFailed = errors.New("image_decode_failed")
)

// ValidateImageHeader sniffs the first 512 bytes of r via
// http.DetectContentType and returns the canonical MIME string ("image/png"
// or "image/jpeg") together with a replay reader whose read position is at
// the start of the stream.
//
// The returned replay reader is composed of the already-read header bytes
// prepended to the remainder of r via io.MultiReader so callers can stream
// the full image to ProbeDimensions or disk without seeking.
//
// PDF is explicitly rejected per D-17: the server never accepts PDF uploads;
// the frontend converts PDF→PNG via pdf.js before uploading. Test
// TestImageUpload_RejectsPDF pins this invariant.
//
// The Content-Type header sent by the client is intentionally ignored —
// only the sniffed bytes drive the accept/reject decision (T-05-05-02).
func ValidateImageHeader(r io.Reader) (mime string, replay io.Reader, err error) {
	head := make([]byte, 512)
	n, _ := io.ReadFull(r, head)
	head = head[:n]
	sniffed := http.DetectContentType(head)
	switch sniffed {
	case "image/png":
		return "image/png", io.MultiReader(bytes.NewReader(head), r), nil
	case "image/jpeg":
		return "image/jpeg", io.MultiReader(bytes.NewReader(head), r), nil
	case "application/pdf":
		return "", nil, fmt.Errorf("%w: PDF not accepted — convert client-side via pdf.js (D-17)", ErrUnsupportedMIME)
	default:
		return "", nil, fmt.Errorf("%w: %s", ErrUnsupportedMIME, sniffed)
	}
}

// ProbeDimensions reads just the image header via image.DecodeConfig (no
// full pixel decode) to obtain width and height in pixels. Returns:
//
//   - w, h — the decoded dimensions
//   - replay — an io.Reader rewound to the start of the image (composed of
//     the bytes consumed by DecodeConfig + the unread tail of r)
//   - err — ErrDimensionTooLarge if either axis > MaxDimension, or
//     ErrImageDecodeFailed if the image header cannot be parsed
//
// ProbeDimensions is called BEFORE writing to disk (T-05-05-01): if the
// dimensions exceed the cap the upload is rejected without touching the
// filesystem.
func ProbeDimensions(r io.Reader) (w, h int, replay io.Reader, err error) {
	var buf bytes.Buffer
	cfg, _, decErr := image.DecodeConfig(io.TeeReader(r, &buf))
	if decErr != nil {
		return 0, 0, nil, fmt.Errorf("%w: %v", ErrImageDecodeFailed, decErr)
	}
	if cfg.Width > MaxDimension || cfg.Height > MaxDimension {
		return cfg.Width, cfg.Height, nil, fmt.Errorf(
			"%w: %d×%d exceeds %d px limit", ErrDimensionTooLarge, cfg.Width, cfg.Height, MaxDimension)
	}
	return cfg.Width, cfg.Height, io.MultiReader(&buf, r), nil
}

// extFromMIME maps a validated MIME type to the file extension used when
// writing the image to disk. Only "image/png" and "image/jpeg" reach this
// function (ValidateImageHeader guarantees that).
func extFromMIME(mime string) string {
	if mime == "image/png" {
		return "png"
	}
	return "jpg"
}
