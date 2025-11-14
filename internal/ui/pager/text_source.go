package pager

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

const (
	textPagerChunkSize  = 128 * 1024
	textPagerCacheLines = 512
)

type textPagerSource struct {
	path          string
	encoding      fsutil.UnicodeEncoding
	chunkSize     int
	file          *os.File
	lines         []textLineRecord
	cache         map[int]string
	cacheOrder    []int
	maxCacheLines int
	partialLine   []byte
	partialOffset int64
	nextOffset    int64
	eof           bool
	bomHandled    bool
	charCount     int
}

type textLineRecord struct {
	offset       int64
	length       int
	runeCount    int
	displayWidth int
}

func newTextPagerSource(path string, preview *statepkg.PreviewData) (*textPagerSource, error) {
	if preview == nil {
		return nil, errors.New("missing preview data")
	}
	if preview.TextEncoding == fsutil.EncodingUTF16LE || preview.TextEncoding == fsutil.EncodingUTF16BE {
		return nil, errors.New("streaming not supported for UTF-16 content")
	}

	source := &textPagerSource{
		path:          path,
		encoding:      preview.TextEncoding,
		chunkSize:     textPagerChunkSize,
		cache:         make(map[int]string),
		maxCacheLines: textPagerCacheLines,
		bomHandled:    preview.TextBytesRead > 0,
		nextOffset:    preview.TextBytesRead,
	}

	for i, line := range preview.TextLines {
		if i >= len(preview.TextLineMeta) {
			break
		}
		meta := preview.TextLineMeta[i]
		record := textLineRecord{
			offset:       meta.Offset,
			length:       meta.Length,
			runeCount:    meta.RuneCount,
			displayWidth: meta.DisplayWidth,
		}
		source.lines = append(source.lines, record)
		source.charCount += meta.RuneCount
		source.cacheLine(i, line)
	}

	if len(preview.TextRemainder) > 0 {
		source.partialLine = append([]byte(nil), preview.TextRemainder...)
		source.partialOffset = preview.TextBytesRead - int64(len(preview.TextRemainder))
	}

	if !preview.TextTruncated {
		source.eof = true
	}

	return source, nil
}

func (s *textPagerSource) Close() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
	s.file = nil
}

func (s *textPagerSource) CharCount() int {
	if s == nil {
		return 0
	}
	return s.charCount
}

func (s *textPagerSource) LineCount() int {
	if s == nil {
		return 0
	}
	return len(s.lines)
}

func (s *textPagerSource) EnsureLine(idx int) error {
	if s == nil {
		return nil
	}
	if idx < 0 {
		idx = 0
	}
	for len(s.lines) <= idx {
		if s.eof {
			return nil
		}
		if err := s.readChunk(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
}

func (s *textPagerSource) EnsureAll() error {
	if s == nil {
		return nil
	}
	for !s.eof {
		if err := s.readChunk(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
}

func (s *textPagerSource) Line(idx int) string {
	if s == nil || idx < 0 {
		return ""
	}
	if err := s.EnsureLine(idx); err != nil {
		return fmt.Sprintf("(error reading file: %v)", err)
	}
	if idx >= len(s.lines) {
		return ""
	}
	if text, ok := s.cache[idx]; ok {
		return text
	}
	text, err := s.readLineText(idx)
	if err != nil {
		return fmt.Sprintf("(error reading file: %v)", err)
	}
	s.cacheLine(idx, text)
	return text
}

func (s *textPagerSource) LineWidth(idx int) int {
	if s == nil || idx < 0 {
		return 0
	}
	if err := s.EnsureLine(idx); err != nil {
		return 0
	}
	if idx >= len(s.lines) {
		return 0
	}
	return s.lines[idx].displayWidth
}

func (s *textPagerSource) readChunk() error {
	if s == nil || s.eof {
		return io.EOF
	}
	if s.file == nil {
		file, err := os.Open(s.path)
		if err != nil {
			return err
		}
		s.file = file
	}

	buf := make([]byte, s.chunkSize)
	offset := s.nextOffset
	n, err := s.file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return err
	}
	if n == 0 {
		s.eof = true
		if len(s.partialLine) > 0 {
			s.appendLine(s.partialLine, s.partialOffset)
			s.partialLine = nil
		}
		return io.EOF
	}

	s.nextOffset += int64(n)
	data := buf[:n]
	dataOffset := offset
	if len(s.partialLine) > 0 {
		data = append(s.partialLine, data...)
		dataOffset = s.partialOffset
		s.partialLine = nil
	}

	if !s.bomHandled && s.encoding == fsutil.EncodingUTF8BOM && len(data) >= 3 {
		dataOffset += 3
		data = data[3:]
		s.bomHandled = true
	}

	cursor := 0
	for cursor < len(data) {
		relative := bytes.IndexByte(data[cursor:], '\n')
		if relative == -1 {
			break
		}
		lineBytes := data[cursor : cursor+relative]
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\r' {
			lineBytes = lineBytes[:len(lineBytes)-1]
		}
		startOffset := dataOffset + int64(cursor)
		s.appendLine(lineBytes, startOffset)
		cursor += relative + 1
	}

	if cursor < len(data) {
		s.partialLine = append([]byte(nil), data[cursor:]...)
		s.partialOffset = dataOffset + int64(cursor)
	} else {
		s.partialLine = nil
	}

	if err == io.EOF {
		s.eof = true
		if len(s.partialLine) > 0 {
			s.appendLine(s.partialLine, s.partialOffset)
			s.partialLine = nil
		}
		return io.EOF
	}

	return nil
}

func (s *textPagerSource) appendLine(lineBytes []byte, start int64) {
	text := string(lineBytes)
	expanded := textutil.ExpandTabs(text, textutil.DefaultTabWidth)
	runes := utf8.RuneCountInString(expanded)
	width := textutil.DisplayWidth(expanded)
	record := textLineRecord{
		offset:       start,
		length:       len(lineBytes),
		runeCount:    runes,
		displayWidth: width,
	}
	s.lines = append(s.lines, record)
	s.charCount += runes
	s.cacheLine(len(s.lines)-1, expanded)
}

func (s *textPagerSource) readLineText(idx int) (string, error) {
	if s.file == nil {
		file, err := os.Open(s.path)
		if err != nil {
			return "", err
		}
		s.file = file
	}
	record := s.lines[idx]
	if record.length <= 0 {
		return "", nil
	}

	buf := make([]byte, record.length)
	n, err := s.file.ReadAt(buf, record.offset)
	if err != nil && err != io.EOF {
		return "", err
	}
	return textutil.ExpandTabs(string(buf[:n]), textutil.DefaultTabWidth), nil
}

func (s *textPagerSource) cacheLine(idx int, text string) {
	if s.cache == nil {
		s.cache = make(map[int]string)
	}
	s.cache[idx] = text
	for i, v := range s.cacheOrder {
		if v == idx {
			s.cacheOrder = append(s.cacheOrder[:i], s.cacheOrder[i+1:]...)
			break
		}
	}
	s.cacheOrder = append(s.cacheOrder, idx)
	if len(s.cacheOrder) > s.maxCacheLines {
		evict := s.cacheOrder[0]
		s.cacheOrder = s.cacheOrder[1:]
		delete(s.cache, evict)
	}
}
