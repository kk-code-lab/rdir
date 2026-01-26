package pager

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	textutil "github.com/kk-code-lab/rdir/internal/textutil"
)

func (p *PreviewPager) clipboardAvailable() bool {
	if p == nil {
		return false
	}
	if p.state == nil || !p.state.ClipboardAvailable {
		return false
	}
	if p.clipboardFunc != nil {
		return true
	}
	return len(p.clipboardCmd) > 0
}

func (p *PreviewPager) recordCopyResult(err error, successMsg string, style string) {
	if err != nil {
		p.setStatusMessage(err.Error(), statusErrorStyle)
		if p.state != nil {
			p.state.LastError = err
		}
		return
	}
	if style == "" {
		style = statusSuccessStyle
	}
	p.setStatusMessage(successMsg, style)
	if p.state != nil {
		p.state.LastYankTime = time.Now()
	}
}

func (p *PreviewPager) copyVisibleToClipboard() error {
	lines := p.visibleContentLinesForCopy()
	return p.copyLinesToClipboard(lines)
}

func (p *PreviewPager) copyAllToClipboard() (string, string, error) {
	if !p.clipboardAvailable() {
		return "", "", errors.New("clipboard unavailable")
	}

	size := p.clipboardByteSize()
	if size > 0 && size >= clipboardHardLimitBytes {
		return "", "", fmt.Errorf("copy canceled: %s exceeds clipboard limit (%s)", formatSize(size), formatSize(clipboardHardLimitBytes))
	}

	if !p.showFormatted && p.rawTextSource != nil {
		if err := p.rawTextSource.EnsureAll(); err != nil {
			return "", "", err
		}
	}

	warn := size > 0 && size >= clipboardWarnBytes
	if warn {
		p.setStatusMessage(fmt.Sprintf("copying %s; this may be slow", formatSize(size)), statusWarnStyle)
	}

	preferRawCopy := p.showFormatted && !p.binaryMode && (p.rawTextSource != nil || len(p.rawLines) > 0)

	if p.clipboardFunc != nil {
		var builder strings.Builder
		if preferRawCopy {
			if err := p.writeAllLinesRaw(&builder); err != nil {
				return "", "", err
			}
		} else {
			if err := p.writeAllLines(&builder); err != nil {
				return "", "", err
			}
		}
		if err := p.clipboardFunc(builder.String()); err != nil {
			return "", "", err
		}
	} else {
		if preferRawCopy {
			if err := p.streamAllLinesToClipboardRaw(); err != nil {
				return "", "", err
			}
		} else {
			if err := p.streamAllLinesToClipboard(); err != nil {
				return "", "", err
			}
		}
	}

	msg := "copied all"
	if preferRawCopy {
		msg = "copied all (raw)"
	}
	if size > 0 {
		msg = fmt.Sprintf("%s (%s)", msg, formatSize(size))
	}
	if warn {
		return msg, statusWarnStyle, nil
	}
	return msg, "", nil
}

func (p *PreviewPager) visibleContentLines() []string {
	if p == nil {
		return nil
	}
	if p.state == nil {
		return nil
	}
	width := p.width
	if width <= 0 {
		width = 1
	}
	height := p.height
	if height <= 0 {
		height = 1
	}
	headerRows := len(p.headerLines())
	if headerRows >= height {
		headerRows = height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}

	contentRows := height - headerRows - 1
	if contentRows < 1 {
		contentRows = 1
	}

	totalLines := p.lineCount()
	p.clampScroll(totalLines, contentRows)

	start := p.state.PreviewScrollOffset
	if start < 0 {
		start = 0
	}
	if totalLines > 0 && start > totalLines {
		start = totalLines
	}
	skipRows := 0
	if p.wrapEnabled {
		skipRows = p.state.PreviewWrapOffset
	}

	rowsRemaining := contentRows
	lines := []string{}
	for i := start; i < totalLines && rowsRemaining > 0; i++ {
		text := lineForClipboard(p.lineAt(i))
		if p.wrapEnabled {
			segments := wrapLineSegments(text, width)
			if skipRows > 0 {
				if skipRows >= len(segments) {
					skipRows = 0
					continue
				}
				segments = segments[skipRows:]
				skipRows = 0
			}
			for _, segment := range segments {
				lines = append(lines, segment)
				rowsRemaining--
				if rowsRemaining == 0 {
					break
				}
			}
			continue
		}

		if width > 0 {
			text = truncateToWidth(text, width)
		}
		lines = append(lines, text)
		rowsRemaining--
	}
	return lines
}

func (p *PreviewPager) visibleContentLinesForCopy() []string {
	if p == nil {
		return nil
	}
	if p.state == nil {
		return nil
	}
	if !p.wrapEnabled || p.width <= 0 {
		return p.visibleContentLines()
	}
	height := p.height
	if height <= 0 {
		height = 1
	}
	headerRows := len(p.headerLines())
	if headerRows >= height {
		headerRows = height - 1
		if headerRows < 0 {
			headerRows = 0
		}
	}

	contentRows := height - headerRows - 1
	if contentRows < 1 {
		contentRows = 1
	}

	totalLines := p.lineCount()
	p.clampScroll(totalLines, contentRows)

	start := p.state.PreviewScrollOffset
	if start < 0 {
		start = 0
	}
	if totalLines > 0 && start > totalLines {
		start = totalLines
	}
	skipRows := p.state.PreviewWrapOffset

	rowsRemaining := contentRows
	lines := []string{}
	for i := start; i < totalLines && rowsRemaining > 0; i++ {
		text := lineForClipboard(p.lineAt(i))
		segments := p.wrapSegmentsRangeForLine(i, text, skipRows, rowsRemaining)
		if len(segments) == 0 {
			skipRows = 0
			continue
		}
		lines = append(lines, strings.Join(segments, ""))
		rowsRemaining -= len(segments)
		skipRows = 0
	}
	return lines
}

func (p *PreviewPager) copyLinesToClipboard(lines []string) error {
	if !p.clipboardAvailable() {
		return errors.New("clipboard unavailable")
	}
	content := strings.Join(lines, "\n")
	if p.clipboardFunc != nil {
		return p.clipboardFunc(content)
	}
	if len(p.clipboardCmd) == 0 {
		return errors.New("clipboard unavailable")
	}
	cmd := clipboardCommand(p.clipboardCmd[0], p.clipboardCmd[1:]...)
	if cmd.Stdin == nil {
		cmd.Stdin = strings.NewReader(content)
	}
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", p.clipboardCmd[0], err)
	}
	return nil
}

func (p *PreviewPager) writeAllLines(w io.Writer) error {
	if p == nil {
		return errors.New("pager unavailable")
	}
	total := p.lineCount()
	if total < 0 {
		total = 0
	}
	bufw := bufio.NewWriter(w)
	for i := 0; i < total; i++ {
		if _, err := bufw.WriteString(lineForClipboard(p.lineAt(i))); err != nil {
			return err
		}
		if i+1 < total {
			if err := bufw.WriteByte('\n'); err != nil {
				return err
			}
		}
	}
	if err := bufw.Flush(); err != nil {
		return err
	}
	return nil
}

func (p *PreviewPager) writeAllLinesRaw(w io.Writer) error {
	if p == nil {
		return errors.New("pager unavailable")
	}
	bufw := bufio.NewWriter(w)
	if p.rawTextSource != nil {
		if err := p.rawTextSource.EnsureAll(); err != nil {
			return err
		}
		total := p.rawTextSource.LineCount()
		if total < 0 {
			total = 0
		}
		for i := 0; i < total; i++ {
			if _, err := bufw.WriteString(lineForClipboard(p.rawTextSource.Line(i))); err != nil {
				return err
			}
			if i+1 < total {
				if err := bufw.WriteByte('\n'); err != nil {
					return err
				}
			}
		}
		return bufw.Flush()
	}

	total := len(p.rawLines)
	for i := 0; i < total; i++ {
		if _, err := bufw.WriteString(lineForClipboard(p.rawLines[i])); err != nil {
			return err
		}
		if i+1 < total {
			if err := bufw.WriteByte('\n'); err != nil {
				return err
			}
		}
	}
	return bufw.Flush()
}

func (p *PreviewPager) streamAllLinesToClipboard() error {
	if len(p.clipboardCmd) == 0 {
		return errors.New("clipboard unavailable")
	}
	cmd := clipboardCommand(p.clipboardCmd[0], p.clipboardCmd[1:]...)
	reader, writer := io.Pipe()
	cmd.Stdin = reader
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	writeErrCh := make(chan error, 1)
	go func() {
		err := p.writeAllLines(writer)
		_ = writer.CloseWithError(err)
		writeErrCh <- err
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", p.clipboardCmd[0], err)
	}
	if err := <-writeErrCh; err != nil {
		return err
	}
	return nil
}

func (p *PreviewPager) streamAllLinesToClipboardRaw() error {
	if len(p.clipboardCmd) == 0 {
		return errors.New("clipboard unavailable")
	}
	cmd := clipboardCommand(p.clipboardCmd[0], p.clipboardCmd[1:]...)
	reader, writer := io.Pipe()
	cmd.Stdin = reader
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	writeErrCh := make(chan error, 1)
	go func() {
		err := p.writeAllLinesRaw(writer)
		_ = writer.CloseWithError(err)
		writeErrCh <- err
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard command %q failed: %w", p.clipboardCmd[0], err)
	}
	if err := <-writeErrCh; err != nil {
		return err
	}
	return nil
}

func (p *PreviewPager) clipboardByteSize() int64 {
	if p == nil || p.state == nil || p.state.PreviewData == nil {
		return 0
	}
	// Prefer the stat size when available; fall back to binary source totals.
	if p.state.PreviewData.Size > 0 {
		return p.state.PreviewData.Size
	}
	if p.binarySource != nil && p.binarySource.totalBytes > 0 {
		return p.binarySource.totalBytes
	}
	return 0
}

func lineForClipboard(text string) string {
	if text == "" {
		return ""
	}
	stripped := stripANSICodes(text)
	return textutil.SanitizeTerminalText(stripped)
}

func stripANSICodes(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
			j := i + 2
			for j < len(text) && text[j] != 'm' {
				j++
			}
			if j < len(text) {
				j++
			}
			i = j
			continue
		}
		ru, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			i++
			continue
		}
		b.WriteRune(ru)
		i += size
	}
	return b.String()
}
