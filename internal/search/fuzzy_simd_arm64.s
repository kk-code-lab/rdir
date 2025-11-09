//go:build arm64 && !purego

#include "textflag.h"

TEXT ·lowerASCIIASIMD(SB), NOSPLIT, $0-24
	MOVD	dst+0(FP), R0
	MOVD	src+8(FP), R1
	MOVD	n+16(FP), R2

	CBZ	R2, done

	VMOVI	$0x41, V16.B16
	VMOVI	$0x5A, V17.B16
	VMOVI	$0x20, V18.B16

loop:
	CMP	$16, R2
	BLO	tail

	VLD1.P	(R1), [V0.B16]
	WORD	$0x4E303C01	// cmge.16b v1, v0, v16
	WORD	$0x4E203E22	// cmge.16b v2, v17, v0
	VAND	V1.B16, V2.B16, V1.B16
	VAND	V1.B16, V18.B16, V3.B16
	VADD	V0.B16, V3.B16, V0.B16

	WORD	$0x2F08A404	// ushll.8h v4, v0, #0
	WORD	$0x6F08A405	// ushll2.8h v5, v0, #0
	WORD	$0x2F10A486	// ushll.4s v6, v4, #0
	WORD	$0x6F10A487	// ushll2.4s v7, v4, #0
	WORD	$0x2F10A4A8	// ushll.4s v8, v5, #0
	WORD	$0x6F10A4A9	// ushll2.4s v9, v5, #0

	VST1.P	[V6.S4], 16(R0)
	VST1.P	[V7.S4], 16(R0)
	VST1.P	[V8.S4], 16(R0)
	VST1.P	[V9.S4], 16(R0)

	SUBS	$16, R2, R2
	BHS	loop

tail:
	CBZ	R2, done

tail_loop:
	MOVBU	(R1), R3
	ADD	$1, R1
	CMP	$'A', R3
	BLT	no_lower
	CMP	$'Z', R3
	BGT	no_lower
	ADD	$0x20, R3
no_lower:
	MOVW	R3, (R0)
	ADD	$4, R0
	SUBS	$1, R2, R2
	BNE	tail_loop

done:
	RET
