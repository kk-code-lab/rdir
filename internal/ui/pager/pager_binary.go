package pager

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (p *PreviewPager) binaryBytesPerLine() int {
	if p == nil || p.binarySource == nil || p.binarySource.bytesPerLine <= 0 {
		return binaryPreviewLineWidth
	}
	return p.binarySource.bytesPerLine
}

func (p *PreviewPager) syncBinaryByteOffsetFromScroll() {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	bytesPerLine := p.binaryBytesPerLine()
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	if p.state.PreviewScrollOffset < 0 {
		p.state.PreviewScrollOffset = 0
	}
	p.state.PreviewBinaryByteOffset = int64(p.state.PreviewScrollOffset) * int64(bytesPerLine)
}

func (p *PreviewPager) syncBinaryScrollFromByteOffset(byteOffset int64) {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	if byteOffset < 0 {
		byteOffset = 0
	}
	bytesPerLine := p.binaryBytesPerLine()
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	p.state.PreviewScrollOffset = int(byteOffset / int64(bytesPerLine))
	p.state.PreviewWrapOffset = 0
}

func (p *PreviewPager) syncBinaryPositionOnEnter() {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	byteOffset := p.state.PreviewBinaryByteOffset
	if byteOffset <= 0 && p.state.PreviewScrollOffset > 0 {
		// The inline (non-pager) binary preview uses the fixed hexdump width.
		byteOffset = int64(p.state.PreviewScrollOffset) * int64(binaryPreviewLineWidth)
	}
	p.state.PreviewBinaryByteOffset = byteOffset
	p.syncBinaryScrollFromByteOffset(byteOffset)
}

func (p *PreviewPager) syncBinaryPositionOnExit() {
	if p == nil || p.state == nil || !p.binaryMode {
		return
	}
	p.state.PreviewWrapOffset = 0

	// Refresh the inline binary preview window around the last byte offset so the
	// non-fullscreen panel doesn't end up showing only the last cached line.
	if p.state.PreviewData != nil && p.binarySource != nil && p.state.CurrentPath != "" && p.state.PreviewData.Name != "" {
		filePath := filepath.Join(p.state.CurrentPath, p.state.PreviewData.Name)
		p.refreshInlineBinaryPreview(filePath, p.binarySource.totalBytes, p.state.PreviewBinaryByteOffset)
		p.state.PreviewScrollOffset = 0
		return
	}

	// Inline preview only has a lightweight set of hexdump lines; if we leave the
	// scroll position pointing beyond that slice, the panel will render empty.
	if p.state.PreviewData != nil && len(p.state.PreviewData.BinaryInfo.Lines) > 0 {
		max := len(p.state.PreviewData.BinaryInfo.Lines) - 1
		if max < 0 {
			max = 0
		}
		if p.state.PreviewScrollOffset > max {
			p.state.PreviewScrollOffset = max
		}
		if p.state.PreviewScrollOffset < 0 {
			p.state.PreviewScrollOffset = 0
		}
		return
	}
	if p.state.PreviewScrollOffset < 0 {
		p.state.PreviewScrollOffset = 0
	}
}

func (p *PreviewPager) refreshInlineBinaryPreview(path string, totalBytes int64, byteOffset int64) {
	if p == nil || p.state == nil || p.state.PreviewData == nil || path == "" || totalBytes <= 0 {
		return
	}

	const maxBytes = 1024
	bytesPerLine := binaryPreviewLineWidth

	start := byteOffset
	if start < 0 {
		start = 0
	}
	start = (start / int64(bytesPerLine)) * int64(bytesPerLine)
	if start >= totalBytes {
		start = totalBytes - 1
		if start < 0 {
			start = 0
		}
		start = (start / int64(bytesPerLine)) * int64(bytesPerLine)
	}

	readLen := int64(maxBytes)
	if remaining := totalBytes - start; remaining < readLen {
		readLen = remaining
	}
	if readLen <= 0 {
		return
	}

	file := p.binarySource.file
	closeFile := false
	if file == nil {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		file = f
		closeFile = true
	}
	if closeFile {
		defer func() { _ = file.Close() }()
	}

	buf := make([]byte, readLen)
	n, err := file.ReadAt(buf, start)
	if n <= 0 {
		return
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return
	}
	buf = buf[:n]

	lines := make([]string, 0, (len(buf)+bytesPerLine-1)/bytesPerLine+2)
	if start > 0 {
		lines = append(lines, fmt.Sprintf("… (showing from %s)", formatHexOffset(start)))
	}
	for off := 0; off < len(buf); off += bytesPerLine {
		end := off + bytesPerLine
		if end > len(buf) {
			end = len(buf)
		}
		lines = append(lines, formatHexLine(int(start)+off, buf[off:end], bytesPerLine))
	}
	if tail := totalBytes - (start + int64(len(buf))); tail > 0 {
		lines = append(lines, fmt.Sprintf("… (%d bytes not shown)", tail))
	}

	p.state.PreviewData.BinaryInfo.Lines = lines
	p.state.PreviewData.BinaryInfo.ByteCount = len(buf)
	p.state.PreviewData.BinaryInfo.TotalBytes = totalBytes
}

func (p *PreviewPager) jumpBinary(deltaBytes int64, stepBytes int64) {
	if p == nil || !p.binaryMode || p.state == nil {
		return
	}
	bytesPerLine := binaryPreviewLineWidth
	if p.binarySource != nil && p.binarySource.bytesPerLine > 0 {
		bytesPerLine = p.binarySource.bytesPerLine
	}
	totalBytes := int64(0)
	if p.binarySource != nil && p.binarySource.totalBytes > 0 {
		totalBytes = p.binarySource.totalBytes
	} else if p.state.PreviewData != nil {
		totalBytes = p.state.PreviewData.BinaryInfo.TotalBytes
		if totalBytes == 0 {
			totalBytes = p.state.PreviewData.Size
		}
	}
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	currentOffset := int64(p.state.PreviewScrollOffset) * int64(bytesPerLine)
	target := currentOffset + deltaBytes

	maxOffset := totalBytes - int64(bytesPerLine)
	if maxOffset < 0 {
		maxOffset = 0
	}
	clamped := false
	if target < 0 {
		target = 0
		clamped = true
	}
	if totalBytes > 0 && target > maxOffset {
		target = maxOffset
		clamped = true
	}

	newLine := int(target / int64(bytesPerLine))
	if newLine < 0 {
		newLine = 0
	}
	p.state.PreviewScrollOffset = newLine
	p.state.PreviewWrapOffset = 0

	applied := target - currentOffset
	if applied != 0 || clamped {
		percent := p.binaryProgressPercent(target, totalBytes)
		var direction string
		if applied > 0 {
			direction = "+"
		} else if applied < 0 {
			direction = "-"
		} else {
			direction = ""
		}
		step := stepBytes
		if step < 0 {
			step = -step
		}
		sizeLabel := fmt.Sprintf("%.0f KB", float64(step)/1024.0)
		msg := fmt.Sprintf("jumped %s%s → %s (%d%%)", direction, sizeLabel, formatHexOffset(target), percent)
		if clamped && applied == 0 {
			msg = "at file boundary"
		}
		p.setStatusMessage(msg, "")
	}
}

func (p *PreviewPager) binaryProgressPercent(offset, total int64) int {
	if total <= 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		offset = total - 1
	}
	return int((offset * 100) / total)
}

func calculateBytesPerLine(pagerWidth int) int {
	// Calculate optimal bytes per line based on available pager width
	// Format: [8-char offset] [hex bytes] |[ASCII bytes]|
	// Minimum width needed for different byte counts:
	// 8 bytes: 10 (offset) + 8*3 (hex) + 3 (spaces) + 3 (separators) + 8 (ASCII) = 48 chars
	// 16 bytes: 10 (offset) + 16*3 (hex) + 4 (spaces) + 3 (separators) + 16 (ASCII) = 81 chars
	// 24 bytes: 10 (offset) + 24*3 (hex) + 5 (spaces) + 3 (separators) + 24 (ASCII) = 112 chars

	if pagerWidth <= 0 {
		return binaryPreviewLineWidth // fallback to default
	}

	// Use the full width for calculation, be more generous with space usage
	// Most terminals can handle tight layouts, and the separators provide visual structure
	if pagerWidth >= 120 {
		return 24 // 24 bytes per line for wide terminals
	} else if pagerWidth >= 90 {
		return 16 // 16 bytes per line for medium terminals
	} else if pagerWidth >= 60 {
		return 8 // 8 bytes per line for narrow terminals
	} else {
		return binaryPreviewLineWidth // fallback to default if very narrow
	}
}

func formatHexOffset(offset int64) string {
	if offset < 0 {
		offset = 0
	}
	s := fmt.Sprintf("%08X", offset)
	var b strings.Builder
	b.Grow(len(s) + len(s)/4 + 2)
	b.WriteString("0x")
	for i, ch := range s {
		if i > 0 && (len(s)-i)%4 == 0 {
			b.WriteByte('_')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func formatHexLine(offset int, chunk []byte, bytesPerLine int) string {
	var builder strings.Builder
	// Estimate buffer size: 10 (offset) + bytesPerLine*3 (hex) + bytesPerLine/8 (spaces) + 3 (separators) + bytesPerLine (ASCII)
	builder.Grow(10 + bytesPerLine*3 + bytesPerLine/8 + 3 + bytesPerLine)
	fmt.Fprintf(&builder, "%08X  ", offset)

	for i := 0; i < bytesPerLine; i++ {
		if i < len(chunk) {
			fmt.Fprintf(&builder, "%02X ", chunk[i])
		} else {
			builder.WriteString("   ")
		}
		// Add grouping space every 8 bytes (after byte 7, 15, 23, etc.)
		if (i+1)%8 == 0 && i < bytesPerLine-1 {
			builder.WriteString(" ")
		}
	}

	builder.WriteString(" |")
	for i := 0; i < len(chunk); i++ {
		builder.WriteByte(printableASCII(chunk[i]))
	}
	for i := len(chunk); i < bytesPerLine; i++ {
		builder.WriteByte(' ')
	}
	builder.WriteString("|")
	return builder.String()
}

func printableASCII(b byte) byte {
	if b >= 32 && b <= 126 {
		return b
	}
	return '.'
}

func alignedBinaryChunkSize(bytesPerLine int) int {
	if bytesPerLine <= 0 {
		bytesPerLine = binaryPreviewLineWidth
	}
	size := binaryPagerChunkSize
	if size <= 0 {
		size = bytesPerLine
	}
	if size < bytesPerLine {
		size = bytesPerLine
	}
	aligned := (size / bytesPerLine) * bytesPerLine
	if aligned < bytesPerLine {
		aligned = bytesPerLine
	}
	return aligned
}

type binaryPagerSource struct {
	path         string
	totalBytes   int64
	bytesPerLine int
	chunkSize    int
	maxChunks    int
	file         *os.File
	cache        map[int]*binaryChunk
	cacheOrder   []int
}

type binaryChunk struct {
	index int
	lines []string
}

func newBinaryPagerSource(path string, totalBytes int64, pagerWidth int) (*binaryPagerSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	bytesPerLine := calculateBytesPerLine(pagerWidth)

	source := &binaryPagerSource{
		path:         path,
		totalBytes:   totalBytes,
		bytesPerLine: bytesPerLine,
		chunkSize:    alignedBinaryChunkSize(bytesPerLine),
		maxChunks:    binaryPagerMaxChunks,
		file:         file,
		cache:        make(map[int]*binaryChunk),
	}
	return source, nil
}

func (s *binaryPagerSource) Close() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
	s.file = nil
}

func (s *binaryPagerSource) UpdateBytesPerLine(pagerWidth int) {
	if s == nil {
		return
	}
	newBytesPerLine := calculateBytesPerLine(pagerWidth)
	if newBytesPerLine == s.bytesPerLine {
		return // no change needed
	}

	// Clear cache since line formatting will change
	s.cache = make(map[int]*binaryChunk)
	s.cacheOrder = nil
	s.bytesPerLine = newBytesPerLine
	s.chunkSize = alignedBinaryChunkSize(newBytesPerLine)
}

func (s *binaryPagerSource) LineCount() int {
	if s == nil {
		return 0
	}
	return s.dataLineCount()
}

func (s *binaryPagerSource) dataLineCount() int {
	if s == nil || s.bytesPerLine <= 0 || s.totalBytes <= 0 {
		return 0
	}
	return int((s.totalBytes + int64(s.bytesPerLine) - 1) / int64(s.bytesPerLine))
}

func (s *binaryPagerSource) Line(idx int) string {
	if s == nil {
		return ""
	}
	if idx < 0 || idx >= s.dataLineCount() {
		return ""
	}
	line, _ := s.lineForDataIndex(idx)
	return line
}

func (s *binaryPagerSource) lineForDataIndex(idx int) (string, error) {
	chunkLines := s.linesPerChunk()
	if chunkLines <= 0 {
		chunkLines = 1
	}
	chunkIndex := idx / chunkLines
	lineOffset := idx % chunkLines
	chunk, err := s.loadChunk(chunkIndex)
	if err != nil {
		return fmt.Sprintf("(error reading file: %v)", err), err
	}
	if chunk == nil || lineOffset >= len(chunk.lines) {
		return "", nil
	}
	return chunk.lines[lineOffset], nil
}

func (s *binaryPagerSource) linesPerChunk() int {
	if s.chunkSize <= 0 || s.bytesPerLine <= 0 {
		return 1
	}
	return s.chunkSize / s.bytesPerLine
}

func (s *binaryPagerSource) loadChunk(index int) (*binaryChunk, error) {
	if chunk, ok := s.cache[index]; ok {
		s.touchChunk(index)
		return chunk, nil
	}
	if s.file == nil {
		file, err := os.Open(s.path)
		if err != nil {
			return nil, err
		}
		s.file = file
	}

	buf := make([]byte, s.chunkSize)
	offset := int64(index) * int64(s.chunkSize)
	n, err := s.file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}
	buf = buf[:n]
	lines := make([]string, 0, (n+s.bytesPerLine-1)/s.bytesPerLine)
	for i := 0; i < n; i += s.bytesPerLine {
		end := i + s.bytesPerLine
		if end > n {
			end = n
		}
		absOffset := int(offset) + i
		lines = append(lines, formatHexLine(absOffset, buf[i:end], s.bytesPerLine))
	}
	chunk := &binaryChunk{
		index: index,
		lines: lines,
	}
	s.addChunk(index, chunk)
	return chunk, nil
}

func (s *binaryPagerSource) addChunk(index int, chunk *binaryChunk) {
	if s.cache == nil {
		s.cache = make(map[int]*binaryChunk)
	}
	s.cache[index] = chunk
	s.touchChunk(index)
	if len(s.cache) > s.maxChunks {
		evict := s.cacheOrder[0]
		s.cacheOrder = s.cacheOrder[1:]
		delete(s.cache, evict)
	}
}

func (s *binaryPagerSource) touchChunk(index int) {
	for i, v := range s.cacheOrder {
		if v == index {
			s.cacheOrder = append(s.cacheOrder[:i], s.cacheOrder[i+1:]...)
			break
		}
	}
	s.cacheOrder = append(s.cacheOrder, index)
}
