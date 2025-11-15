package state

type markdownDocument struct {
	blocks []markdownBlock
}

type markdownBlock interface {
	blockType() markdownBlockType
}

type markdownBlockType int

const (
	blockParagraph markdownBlockType = iota
	blockHeading
	blockCode
	blockList
	blockBlockquote
	blockHorizontalRule
	blockTable
)

type markdownInlineType int

const (
	inlineText markdownInlineType = iota
	inlineEmphasis
	inlineStrong
	inlineStrike
	inlineCode
	inlineLink
	inlineImage
	inlineLineBreak
)

type markdownInline struct {
	kind        markdownInlineType
	literal     string
	children    []markdownInline
	destination string
}

type markdownHeading struct {
	level int
	text  []markdownInline
}

func (markdownHeading) blockType() markdownBlockType { return blockHeading }

type markdownParagraph struct {
	text []markdownInline
}

func (markdownParagraph) blockType() markdownBlockType { return blockParagraph }

type markdownCodeBlock struct {
	info     string
	lines    []string
	fenced   bool
	indented bool
}

func (markdownCodeBlock) blockType() markdownBlockType { return blockCode }

type markdownList struct {
	ordered bool
	start   int
	items   []markdownListItem
}

type markdownListItem struct {
	blocks []markdownBlock
}

func (markdownList) blockType() markdownBlockType { return blockList }

type markdownBlockquote struct {
	blocks []markdownBlock
}

func (markdownBlockquote) blockType() markdownBlockType { return blockBlockquote }

type markdownHorizontalRule struct{}

func (markdownHorizontalRule) blockType() markdownBlockType { return blockHorizontalRule }

type markdownTable struct {
	headers []string
	rows    [][]string
	align   []tableAlignment
}

func (markdownTable) blockType() markdownBlockType { return blockTable }

type tableAlignment int

const (
	alignDefault tableAlignment = iota
	alignLeft
	alignCenter
	alignRight
)
