package fuzz

import (
	"sort"
	"unicode/utf16"
	"unicode/utf8"
)

// ReplacePayloads injects payloads[i] into positions[i] of raw, returning the
// new string. Positions are sorted by Start internally so the caller may pass
// them in any order. Overlap is not validated here (Validate handles that at
// the API boundary); if overlapping positions slip through, the result is
// undefined but the function does not panic.
//
// Position semantics: Start and End are UTF-16 code unit offsets into raw,
// matching the values produced by browser textarea selectionStart/selectionEnd
// (which the UI sends). They are converted to UTF-8 byte offsets internally
// before slicing so that non-ASCII characters (notably the § insertion marker)
// do not corrupt the substitution.
//
// Length contract: len(payloads) must equal len(positions). The caller is
// responsible for ensuring this; mode strategies that yield assignments
// satisfy it by construction.
func ReplacePayloads(raw string, positions []FuzzerPosition, payloads []string) string {
	if len(positions) == 0 || len(payloads) != len(positions) {
		return raw
	}
	// Sort an index permutation, not the slice itself, so payloads[i] still
	// refers to positions[i] from the caller's perspective.
	order := make([]int, len(positions))
	for i := range positions {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return positions[order[a]].Start < positions[order[b]].Start
	})

	// Convert all positions from UTF-16 code unit offsets to byte offsets
	// against the original raw string once, so the slicing loop can use the
	// proven running-offset adjustment to handle replacements of different
	// length than the substring they replace.
	type byteRange struct {
		start, end int
		ok         bool
	}
	byteRanges := make([]byteRange, len(positions))
	for _, idx := range order {
		p := positions[idx]
		startByte, ok := utf16OffsetToByte(raw, p.Start)
		if !ok {
			byteRanges[idx] = byteRange{ok: false}
			continue
		}
		endByte, ok := utf16OffsetToByte(raw, p.End)
		if !ok {
			byteRanges[idx] = byteRange{ok: false}
			continue
		}
		byteRanges[idx] = byteRange{start: startByte, end: endByte, ok: true}
	}

	// Walk the sorted positions, tracking byte offset adjustment as
	// replacements shift content downstream.
	out := raw
	offset := 0
	for _, idx := range order {
		br := byteRanges[idx]
		if !br.ok {
			continue
		}
		payload := payloads[idx]
		start := br.start + offset
		end := br.end + offset
		if start < 0 || end > len(out) || start > end {
			continue
		}
		out = out[:start] + payload + out[end:]
		offset += len(payload) - (br.end - br.start)
	}
	return out
}

// utf16OffsetToByte converts a UTF-16 code unit offset into the byte offset of
// the same logical position in s (UTF-8). Returns false if the offset is out
// of range or falls inside a multi-unit rune. An offset equal to the UTF-16
// length of s maps to len(s).
func utf16OffsetToByte(s string, u16Offset int) (int, bool) {
	if u16Offset < 0 {
		return 0, false
	}
	if u16Offset == 0 {
		return 0, true
	}
	u16Pos := 0
	bytePos := 0
	for bytePos < len(s) {
		r, size := utf8.DecodeRuneInString(s[bytePos:])
		w := 1
		if utf16.IsSurrogate(r) || r > 0xFFFF {
			w = 2
		}
		if u16Pos+w > u16Offset {
			return 0, false
		}
		u16Pos += w
		bytePos += size
		if u16Pos == u16Offset {
			return bytePos, true
		}
	}
	if u16Pos == u16Offset {
		return bytePos, true
	}
	return 0, false
}
