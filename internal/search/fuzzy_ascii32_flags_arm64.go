//go:build arm64 && !purego

package search

import "os"

var ascii32Debug = os.Getenv("RDIR_DEBUG_ASCII32") == "1"
var ascii32PrefixASMDisabled = os.Getenv("RDIR_DISABLE_ASCII32_PREFIX_ASM") == "1"
var ascii32VerifyPrefixASM = os.Getenv("RDIR_VERIFY_ASCII32_PREFIX_ASM") == "1"

const (
	ascii32MaxRows    = 128
	ascii32MaxCols    = 4096
	ascii32BeamWidth  = 96
	ascii32BeamMargin = 48
	ascii32ChunkWidth = 8
)
