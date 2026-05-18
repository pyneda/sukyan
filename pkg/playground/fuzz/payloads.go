package fuzz

import (
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/rs/zerolog/log"
)

// ResolveGroup flattens a FuzzerPayloadsGroup to a concrete []string by
// expanding the inline list, reading the wordlist file (if any), and applying
// the processor chain in order to every entry.
//
// On per-payload processor errors the original payload is skipped and an
// error is logged — matches existing behaviour from the legacy engine to
// avoid surprising users mid-rollout.
//
// Wordlist read errors return an empty slice for the wordlist portion (and
// log); the inline list portion is still returned. Caller is expected to
// validate that the resolved slice is non-empty.
func ResolveGroup(g FuzzerPayloadsGroup) []string {
	out := make([]string, 0, len(g.Payloads))
	processors := buildProcessors(g.Processors)

	// Inline list first, then wordlist contents — match the legacy ordering.
	if len(g.Payloads) > 0 {
		out = appendProcessed(out, g.Payloads, processors)
	}

	if g.Wordlist != "" {
		storage := manual.NewFilesystemWordlistStorage()
		wl, err := storage.GetWordlistByID(g.Wordlist)
		if err != nil {
			log.Error().Err(err).Str("wordlist", g.Wordlist).Msg("fuzz: get wordlist by id")
			return out
		}
		lines, err := storage.ReadWordlist(wl.Name, 0)
		if err != nil {
			log.Error().Err(err).Str("wordlist", g.Wordlist).Msg("fuzz: read wordlist")
			return out
		}
		out = appendProcessed(out, lines, processors)
	}

	return out
}

// ResolvePositionPayloads returns the resolved payload list for a single
// position, concatenating all its groups.
func ResolvePositionPayloads(p FuzzerPosition) []string {
	out := make([]string, 0)
	for _, g := range p.PayloadGroups {
		out = append(out, ResolveGroup(g)...)
	}
	return out
}

// ResolvedPayloads is the engine-internal resolved view used by the mode
// strategies: one element per position (Paired/Combinations) or nil + a
// shared slice (Single/All).
type ResolvedPayloads struct {
	Shared    []string
	PerPosition [][]string
}

// Resolve produces a ResolvedPayloads for the given mode + config. It does
// not validate consistency — that's the caller's job (api layer or Validate
// below).
func Resolve(mode FuzzMode, positions []FuzzerPosition, shared *FuzzerPayloadsGroup) ResolvedPayloads {
	switch mode {
	case ModeSingle, ModeAll:
		if shared == nil {
			return ResolvedPayloads{}
		}
		return ResolvedPayloads{Shared: ResolveGroup(*shared)}
	case ModePaired, ModeCombinations:
		per := make([][]string, len(positions))
		for i, p := range positions {
			per[i] = ResolvePositionPayloads(p)
		}
		return ResolvedPayloads{PerPosition: per}
	}
	return ResolvedPayloads{}
}

func buildProcessors(names []string) []lib.StringProcessor {
	if len(names) == 0 {
		return nil
	}
	out := make([]lib.StringProcessor, 0, len(names))
	for _, n := range names {
		out = append(out, lib.StringProcessor{Type: lib.StringOperation(n)})
	}
	return out
}

func appendProcessed(dst, src []string, processors []lib.StringProcessor) []string {
	if len(processors) == 0 {
		return append(dst, src...)
	}
	for _, s := range src {
		processed, err := lib.ProcessString(s, processors)
		if err != nil {
			log.Error().Err(err).Str("payload", s).Interface("processors", processors).Msg("fuzz: process payload")
			continue
		}
		dst = append(dst, processed)
	}
	return dst
}
