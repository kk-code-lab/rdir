package fs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/unicode"
)

const (
	textDetectionSampleSize      = 4096
	nonPrintableThresholdPercent = 30
)

type unicodeEncoding int

const (
	encodingUnknown unicodeEncoding = iota
	encodingUTF8BOM
	encodingUTF16LE
	encodingUTF16BE
)

var binaryExtensions = map[string]struct{}{
	".7z":    {},
	".apk":   {},
	".avi":   {},
	".bin":   {},
	".bmp":   {},
	".bz2":   {},
	".class": {},
	".dat":   {},
	".dll":   {},
	".doc":   {},
	".docx":  {},
	".dylib": {},
	".exe":   {},
	".flac":  {},
	".gif":   {},
	".gz":    {},
	".ico":   {},
	".iso":   {},
	".jar":   {},
	".jpeg":  {},
	".jpg":   {},
	".mkv":   {},
	".mov":   {},
	".mp3":   {},
	".mp4":   {},
	".ogg":   {},
	".otf":   {},
	".pdf":   {},
	".png":   {},
	".ppt":   {},
	".pptx":  {},
	".psd":   {},
	".so":    {},
	".tar":   {},
	".tgz":   {},
	".ttf":   {},
	".wav":   {},
	".wasm":  {},
	".woff":  {},
	".woff2": {},
	".xls":   {},
	".xlsx":  {},
	".xz":    {},
	".zip":   {},
}

// IsTextFile determines if content is text or binary.
// The path (if provided) is used to short-circuit obvious binary extensions before sniffing.
func IsTextFile(path string, content []byte) bool {
	if looksBinaryByExtension(path) {
		return false
	}

	if len(content) == 0 {
		return true
	}

	sample := content
	if len(sample) > textDetectionSampleSize {
		sample = sample[:textDetectionSampleSize]
	}

	if enc := detectUnicodeEncoding(sample); enc != encodingUnknown {
		return true
	}

	if bytes.IndexByte(sample, 0x00) != -1 {
		return false
	}

	if utf8.Valid(sample) {
		return true
	}

	printable := 0
	nonPrintable := 0
	for _, b := range sample {
		if isCommonTextByte(b) {
			printable++
		} else {
			nonPrintable++
		}
	}

	if printable == 0 {
		return false
	}

	return nonPrintable*100/len(sample) < nonPrintableThresholdPercent
}

// ReadFileHead returns up to limit bytes from the beginning of path.
func ReadFileHead(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	return io.ReadAll(io.LimitReader(f, limit))
}

// ReadTextSample returns a small sample of the file for text/binary sniffing.
func ReadTextSample(path string) ([]byte, error) {
	return ReadFileHead(path, textDetectionSampleSize)
}

func looksBinaryByExtension(path string) bool {
	if path == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := binaryExtensions[ext]
	return ok
}

func isCommonTextByte(b byte) bool {
	switch {
	case b == 0x09 || b == 0x0A || b == 0x0D:
		return true
	case b >= 0x20 && b <= 0x7E:
		return true
	case b == 0x1B:
		return true
	case b >= 0x80:
		return true
	default:
		return false
	}
}

func detectUnicodeEncoding(sample []byte) unicodeEncoding {
	if len(sample) >= 3 && sample[0] == 0xEF && sample[1] == 0xBB && sample[2] == 0xBF {
		return encodingUTF8BOM
	}
	if len(sample) >= 2 {
		switch {
		case sample[0] == 0xFF && sample[1] == 0xFE:
			return encodingUTF16LE
		case sample[0] == 0xFE && sample[1] == 0xFF:
			return encodingUTF16BE
		}
	}
	return encodingUnknown
}

// NormalizeTextContent converts known Unicode BOM-encoded content into UTF-8 strings.
func NormalizeTextContent(content []byte) string {
	if len(content) == 0 {
		return ""
	}

	switch detectUnicodeEncoding(content) {
	case encodingUTF8BOM:
		return string(content[3:])
	case encodingUTF16LE:
		return decodeUTF16(content, unicode.LittleEndian)
	case encodingUTF16BE:
		return decodeUTF16(content, unicode.BigEndian)
	default:
		return string(content)
	}
}

func decodeUTF16(content []byte, endian unicode.Endianness) string {
	decoder := unicode.UTF16(endian, unicode.ExpectBOM).NewDecoder()
	out, err := decoder.Bytes(content)
	if err != nil {
		return string(content)
	}
	return string(out)
}
