// Package detect identifies files by their contents rather than their name.
//
// Extensions are wrong often enough to matter: phones share HEIC as .jpg, a
// failed download saves an HTML error page as .pdf, and a renamed legacy .doc
// claims to be .docx. Reading the bytes lets Lathe say "this is actually a HEIC
// image" instead of failing with a decoder error.
package detect

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

// Category groups formats by what a user can do with them. Task inputs are
// matched against categories, never against extensions.
type Category string

// The categories Lathe understands.
const (
	CategoryPDF      Category = "pdf"
	CategoryImage    Category = "image"
	CategoryDocument Category = "document"
	CategoryVideo    Category = "video"
	CategoryAudio    Category = "audio"
	CategoryText     Category = "text"
	CategoryArchive  Category = "archive"
	CategoryUnknown  Category = "unknown"
)

// FileType is what Lathe knows about a file after looking inside it.
type FileType struct {
	MIME string
	// Extension is the canonical one for the detected type, which may differ
	// from the name on disk.
	Extension  string
	Category   Category
	Confidence float64
	SizeBytes  int64

	// Encrypted is true for a PDF that needs a password. Detecting this up
	// front lets the UI ask for the password instead of failing three steps
	// into a job.
	Encrypted bool

	// Details carries whatever the sniffer could cheaply establish: page
	// count, pixel dimensions, and so on.
	Details map[string]string
}

// String renders the type for logs and technical detail panes.
func (f FileType) String() string {
	return fmt.Sprintf("%s (%s)", f.Extension, f.MIME)
}

// MismatchesName reports whether the file's real type disagrees with its
// extension, which is worth telling the user about.
func (f FileType) MismatchesName(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" || f.Extension == "" || f.Category == CategoryUnknown {
		return false
	}
	for _, alias := range aliases(f.Extension) {
		if ext == alias {
			return false
		}
	}
	return true
}

// ErrEmptyFile reports a zero-byte input, which every engine would otherwise
// fail on in its own idiosyncratic way.
var ErrEmptyFile = errors.New("the file is empty")

// sniffLen is enough for every signature below plus mimetype's own table.
const sniffLen = 8192

// Detect identifies the file at path by content.
func Detect(path string) (FileType, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileType{}, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return FileType{}, fmt.Errorf("read file information: %w", err)
	}
	if info.IsDir() {
		return FileType{}, errors.New("that is a folder, not a file")
	}
	if info.Size() == 0 {
		return FileType{}, ErrEmptyFile
	}

	head := make([]byte, sniffLen)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return FileType{}, fmt.Errorf("read file: %w", err)
	}
	head = head[:n]

	ft := FileType{SizeBytes: info.Size(), Details: map[string]string{}}

	// Signatures the shared table gets wrong or reports too coarsely.
	if special, ok := detectSpecial(head); ok {
		ft.MIME, ft.Extension, ft.Category, ft.Confidence = special.mime, special.ext, special.category, 1
	} else {
		mt := mimetype.Detect(head)
		ft.MIME = mt.String()
		ft.Extension = strings.TrimPrefix(mt.Extension(), ".")
		ft.Category = categoryFor(mt.String(), ft.Extension)
		ft.Confidence = confidenceFor(ft.Category)
	}

	if ft.Category == CategoryPDF {
		describePDF(f, info.Size(), &ft)
	}
	if ft.Category == CategoryImage {
		describeImage(head, &ft)
	}
	return ft, nil
}

// All runs Detect over several paths, reporting the first failure with
// the offending filename attached.
func All(paths []string) ([]FileType, error) {
	out := make([]FileType, 0, len(paths))
	for _, p := range paths {
		ft, err := Detect(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		out = append(out, ft)
	}
	return out, nil
}

type signature struct {
	mime     string
	ext      string
	category Category
}

// detectSpecial covers formats where a plain prefix table is not enough. HEIC
// is the important one: its signature lives at byte 4, inside the ftyp box, so
// prefix-only detectors report a generic MP4 or nothing at all.
func detectSpecial(head []byte) (signature, bool) {
	if len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) {
		switch brand := string(head[8:12]); brand {
		case "heic", "heix", "hevc", "hevx", "mif1", "msf1", "heim", "heis":
			return signature{"image/heic", "heic", CategoryImage}, true
		case "avif", "avis":
			return signature{"image/avif", "avif", CategoryImage}, true
		}
	}
	if len(head) >= 5 && bytes.Equal(head[:5], []byte("%PDF-")) {
		return signature{"application/pdf", "pdf", CategoryPDF}, true
	}
	return signature{}, false
}

func categoryFor(mime, ext string) Category {
	switch {
	case mime == "application/pdf":
		return CategoryPDF
	case strings.HasPrefix(mime, "image/"):
		return CategoryImage
	case strings.HasPrefix(mime, "video/"):
		return CategoryVideo
	case strings.HasPrefix(mime, "audio/"):
		return CategoryAudio
	}
	switch ext {
	case "doc", "docx", "odt", "rtf", "ppt", "pptx", "odp", "xls", "xlsx", "ods", "epub":
		return CategoryDocument
	case "zip", "gz", "bz2", "xz", "7z", "rar", "tar":
		return CategoryArchive
	case "txt", "csv", "md", "json", "xml", "html", "htm":
		return CategoryText
	}
	if strings.HasPrefix(mime, "text/") {
		return CategoryText
	}
	return CategoryUnknown
}

func confidenceFor(c Category) float64 {
	switch c {
	case CategoryUnknown:
		return 0.2
	case CategoryText, CategoryArchive:
		// A ZIP container could be a docx, an ODF file or a plain archive.
		return 0.7
	default:
		return 0.95
	}
}

// aliases lists extensions that legitimately name the same format, so a
// correctly named file is not reported as a mismatch.
func aliases(ext string) []string {
	switch ext {
	case "jpg", "jpeg":
		return []string{"jpg", "jpeg", "jpe"}
	case "tif", "tiff":
		return []string{"tif", "tiff"}
	case "htm", "html":
		return []string{"htm", "html"}
	case "zip":
		// Every OOXML and ODF document is a ZIP underneath.
		return []string{"zip", "docx", "xlsx", "pptx", "odt", "ods", "odp", "epub"}
	case "heic":
		return []string{"heic", "heif"}
	case "mpga", "mp3":
		return []string{"mp3", "mpga"}
	default:
		return []string{ext}
	}
}

// describePDF fills in the page count and whether a password is required.
// It reads the trailer rather than parsing the document, which is cheap enough
// to run on every dropped file.
func describePDF(f *os.File, size int64, ft *FileType) {
	const tailLen = 64 << 10
	start := size - tailLen
	if start < 0 {
		start = 0
	}
	tail := make([]byte, size-start)
	if _, err := f.ReadAt(tail, start); err != nil && !errors.Is(err, io.EOF) {
		return
	}
	if bytes.Contains(tail, []byte("/Encrypt")) {
		ft.Encrypted = true
		ft.Details["encrypted"] = "true"
	}
	if n := bytes.Count(tail, []byte("/Type /Page")) + bytes.Count(tail, []byte("/Type/Page")); n > 0 {
		ft.Details["pagesHint"] = fmt.Sprint(n)
	}
}

// describeImage extracts pixel dimensions from the header for the formats
// where it is a fixed, trivial read. Anything else is left to the engine.
func describeImage(head []byte, ft *FileType) {
	var w, h int
	switch ft.Extension {
	case "png":
		if len(head) >= 24 && bytes.Equal(head[12:16], []byte("IHDR")) {
			w = int(binary.BigEndian.Uint32(head[16:20]))
			h = int(binary.BigEndian.Uint32(head[20:24]))
		}
	case "gif":
		if len(head) >= 10 {
			w = int(binary.LittleEndian.Uint16(head[6:8]))
			h = int(binary.LittleEndian.Uint16(head[8:10]))
		}
	case "bmp":
		if len(head) >= 26 {
			w = int(int32(binary.LittleEndian.Uint32(head[18:22])))
			h = int(int32(binary.LittleEndian.Uint32(head[22:26])))
		}
	case "jpg", "jpeg":
		w, h = jpegSize(head)
	}
	if w > 0 && h > 0 {
		ft.Details["width"] = fmt.Sprint(w)
		ft.Details["height"] = fmt.Sprint(h)
	}
}

// jpegSize walks JPEG markers to the first frame header. It reads only the
// sniff buffer, so a file with a very large EXIF block simply reports nothing.
func jpegSize(head []byte) (w, h int) {
	for i := 2; i+9 < len(head); {
		if head[i] != 0xFF {
			i++
			continue
		}
		marker := head[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		segLen := int(binary.BigEndian.Uint16(head[i+2 : i+4]))
		// SOF0..SOF15, excluding the DHT/JPG/DAC markers interleaved among them.
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			return int(binary.BigEndian.Uint16(head[i+7 : i+9])),
				int(binary.BigEndian.Uint16(head[i+5 : i+7]))
		}
		if segLen < 2 {
			return 0, 0
		}
		i += 2 + segLen
	}
	return 0, 0
}
