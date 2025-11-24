package render

import "github.com/gdamore/tcell/v2"

// ColorTheme defines application colors.
type ColorTheme struct {
	Background      tcell.Color
	Foreground      tcell.Color
	SidebarBg       tcell.Color
	SidebarFg       tcell.Color
	HiddenFg        tcell.Color
	SidebarActiveBg tcell.Color
	SidebarActiveFg tcell.Color
	SelectionBg     tcell.Color
	SelectionFg     tcell.Color
	DirectoryFg     tcell.Color
	SymlinkFg       tcell.Color
	FileFg          tcell.Color
	FooterBg        tcell.Color
	FooterFg        tcell.Color
	PreviewBg       tcell.Color
	PreviewFg       tcell.Color
	CodeBg          tcell.Color
	CodeFg          tcell.Color
	CodeBlockBg     tcell.Color
	CodeBlockFg     tcell.Color
}

// GetColorTheme returns the default color scheme.
func GetColorTheme() ColorTheme {
	return ColorTheme{
		Background:      tcell.ColorDefault,
		Foreground:      tcell.ColorDefault,
		SidebarBg:       tcell.ColorDefault,
		SidebarFg:       tcell.ColorDefault,
		HiddenFg:        tcell.ColorLightSlateGray,
		SidebarActiveBg: tcell.Color33,
		SidebarActiveFg: tcell.ColorWhite,
		SelectionBg:     tcell.Color33,
		SelectionFg:     tcell.ColorWhite,
		DirectoryFg:     tcell.Color33,
		SymlinkFg:       tcell.Color51,
		FileFg:          tcell.ColorDefault,
		FooterBg:        tcell.ColorDefault,
		FooterFg:        tcell.ColorDefault,
		PreviewBg:       tcell.ColorDefault,
		PreviewFg:       tcell.ColorDefault,
		CodeBg:          tcell.ColorDefault,
		CodeFg:          tcell.Color44,  // brighter cyan text for code
		CodeBlockBg:     tcell.Color234, // darker grey background for fenced code
		CodeBlockFg:     tcell.Color252, // light grey text for fenced code
	}
}
