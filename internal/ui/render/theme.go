package render

import "github.com/gdamore/tcell/v2"

// ColorTheme defines application colors.
type ColorTheme struct {
	Background      tcell.Color
	Foreground      tcell.Color
	SidebarBg       tcell.Color
	SidebarFg       tcell.Color
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
}

// GetColorTheme returns the default color scheme.
func GetColorTheme() ColorTheme {
	return ColorTheme{
		Background:      tcell.ColorDefault,
		Foreground:      tcell.ColorDefault,
		SidebarBg:       tcell.ColorDefault,
		SidebarFg:       tcell.ColorDefault,
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
	}
}
