//go:build arm64 && !purego

#include "textflag.h"

#define PREFIX_LANE(vec, lane, skip_label, nocand_label) \
	CMPW	R5, R8 \
	BEQ	skip_label \
	FCMPS	F2, F0 \
	BLE	skip_label \
	FSUBS	F1, F0, F0 \
skip_label: \
	VMOV	R10, vec.S[lane] \
	FMOVS	R10, F6 \
	CBZ	R4, nocand_label \
	FCMPS	F0, F4 \
	BLE	nocand_label \
	FMOVS	F4, F0 \
	SUB	$1, R4, R7 \
	MOVW	R7, R5 \
nocand_label: \
	FMOVS	F0, (R0) \
	MOVW	R5, (R1) \
	ADD	$4, R0, R0 \
	ADD	$4, R1, R1 \
	ADD	$1, R4, R4 \
	SUBS	$1, R3, R3 \
	FMOVS	F6, F4

// Compute prefix window for dpPrev using float32 gap penalties.
TEXT ·fuzzyPrefixMaxASCIIAsm(SB), NOSPLIT|NOFRAME, $0-40
	MOVD	prefix+0(FP), R0     // prefix base
	MOVD	prefixIdx+8(FP), R1  // prefixIdx base
	MOVD	dpPrev+16(FP), R2    // dpPrev base
	MOVD	count+24(FP), R3     // number of columns
	CBZ	R3, prefix_done

	FMOVS	gap+32(FP), F1       // gap penalty (float32)

	MOVW	$-1, R5              // bestIdx
	MOVW	$-1, R8              // constant -1
	MOVW	$0, R4               // column index
	MOVW	$0xFF800000, R6
	FMOVS	R6, F0               // bestScore = -Inf
	FMOVS	R6, F2               // threshold = -Inf
	FMOVS	R6, F4               // prev dpPrev value (-Inf sentinel)

prefix_chunk_loop:
	CMP	$8, R3
	BLT	prefix_tail

	VLD1.P	(R2), [V0.S4]
	VLD1.P	(R2), [V1.S4]

	PREFIX_LANE(V0, 0, prefix_lane0_skipdec, prefix_lane0_nocand)
	PREFIX_LANE(V0, 1, prefix_lane1_skipdec, prefix_lane1_nocand)
	PREFIX_LANE(V0, 2, prefix_lane2_skipdec, prefix_lane2_nocand)
	PREFIX_LANE(V0, 3, prefix_lane3_skipdec, prefix_lane3_nocand)
	PREFIX_LANE(V1, 0, prefix_lane4_skipdec, prefix_lane4_nocand)
	PREFIX_LANE(V1, 1, prefix_lane5_skipdec, prefix_lane5_nocand)
	PREFIX_LANE(V1, 2, prefix_lane6_skipdec, prefix_lane6_nocand)
	PREFIX_LANE(V1, 3, prefix_lane7_skipdec, prefix_lane7_nocand)

	B	prefix_chunk_loop

prefix_tail:
	CBZ	R3, prefix_done

prefix_tail_loop:
	FMOVS	(R2), F6
	ADD	$4, R2, R2

	CMPW	R5, R8
	BEQ	prefix_tail_skipdec
	FCMPS	F2, F0
	BLE	prefix_tail_skipdec
	FSUBS	F1, F0, F0
prefix_tail_skipdec:
	CBZ	R4, prefix_tail_nocand
	FCMPS	F0, F4
	BLE	prefix_tail_nocand
	FMOVS	F4, F0
	SUB	$1, R4, R7
	MOVW	R7, R5
prefix_tail_nocand:
	FMOVS	F0, (R0)
	MOVW	R5, (R1)
	ADD	$4, R0, R0
	ADD	$4, R1, R1
	ADD	$1, R4, R4
	SUBS	$1, R3, R3
	FMOVS	F6, F4
	BGT	prefix_tail_loop

prefix_done:
	RET

#undef PREFIX_LANE

// Tiny helper to verify pointer writes from asm are visible in Go.
// Writes 1.0f to *prefix and -1 to *prefixIdx.
TEXT ·fuzzyWriteOneAsm(SB), NOSPLIT|NOFRAME, $0-16
    MOVD    prefix+0(FP), R0
    MOVD    prefixIdx+8(FP), R1
    MOVW    $0x3f800000, R2
    MOVW    R2, (R0)
    MOVW    $-1, R3
    MOVW    R3, (R1)
    RET

// Set prefixIdx[0:count] = -1
TEXT ·fuzzySetIdxRangeAsm(SB), NOSPLIT|NOFRAME, $0-16
    MOVD    prefixIdx+0(FP), R0   // base
    MOVD    count+8(FP), R1       // count
    MOVW    $-1, R2               // value -1
    CBZ     R1, set_done

// loop over blocks of 8
set_loop8:
    MOVD    R1, R3
    SUBS    $8, R3, R3
    BLT     set_tail

    // NEON: set all bytes to 0xFF (-1 for int32 lanes) and store 8 lanes
    VMOVI   $0xFF, V0.B16
    VST1.P  [V0.S4], 16(R0)
    VST1.P  [V0.S4], 16(R0)

    SUB     $8, R1, R1
    B       set_loop8

// tail loop for remaining 0..7
set_tail:
    CBZ     R1, set_done
set_tail_loop:
    MOVW    R2, (R0)
    ADD     $4, R0, R0
    SUBS    $1, R1, R1
    BGT     set_tail_loop

set_done:
    RET

// Copy src[0:count] -> dst[0:count] (float32)
TEXT ·fuzzyCopyRangeF32Asm(SB), NOSPLIT|NOFRAME, $0-24
    MOVD    dst+0(FP), R0    // dst base
    MOVD    src+8(FP), R1    // src base
    MOVD    count+16(FP), R2 // count
    CBZ     R2, copy_done

// loop blocks of 8 using NEON loads/stores
copy_loop8:
    MOVD    R2, R3
    SUBS    $8, R3, R3
    BLT     copy_tail

    // load 8 float32 from src (2x 128-bit vectors)
    VLD1.P  (R1), [V0.S4]
    VLD1.P  (R1), [V1.S4]
    // store 8 float32 to dst
    VST1.P  [V0.S4], 16(R0)
    VST1.P  [V1.S4], 16(R0)

    SUB     $8, R2, R2
    B       copy_loop8

copy_tail:
    CBZ     R2, copy_done
copy_tail_loop:
    MOVW    (R1), R4
    MOVW    R4, (R0)
    ADD     $4, R0, R0
    ADD     $4, R1, R1
    SUBS    $1, R2, R2
    BGT     copy_tail_loop

copy_done:
    RET

// Constant block: 8 lanes of float32 -Inf (0xFF800000)
DATA ·f32NegInfBlock+0(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+4(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+8(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+12(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+16(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+20(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+24(SB)/4, $0xFF800000
DATA ·f32NegInfBlock+28(SB)/4, $0xFF800000
GLOBL ·f32NegInfBlock(SB), RODATA, $32

// Fill dst[0:count] with float32 -Inf using NEON chunk-8 + tail
TEXT ·fuzzySetRangeF32NegInfAsm(SB), NOSPLIT|NOFRAME, $0-16
    MOVD    dst+0(FP), R0
    MOVD    count+8(FP), R1
    CBZ     R1, setf_done

setf_loop8:
    MOVD    R1, R2
    SUBS    $8, R2, R2
    BLT     setf_tail

    MOVD    $·f32NegInfBlock(SB), R3
    VLD1    (R3), [V0.S4]
    ADD     $16, R3, R3
    VLD1    (R3), [V1.S4]
    VST1.P  [V0.S4], 16(R0)
    VST1.P  [V1.S4], 16(R0)
    SUB     $8, R1, R1
    B       setf_loop8

setf_tail:
    CBZ     R1, setf_done
setf_tail_loop:
    MOVW    $0xFF800000, R4
    MOVW    R4, (R0)
    ADD     $4, R0, R0
    SUBS    $1, R1, R1
    BGT     setf_tail_loop

setf_done:
    RET
