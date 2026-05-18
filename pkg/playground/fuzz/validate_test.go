package fuzz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func mkPos(start, end int, val string) FuzzerPosition {
	return FuzzerPosition{Start: start, End: end, OriginalValue: val}
}

func mkPosWithGroup(start, end int, val string, payloads ...string) FuzzerPosition {
	p := mkPos(start, end, val)
	p.PayloadGroups = []FuzzerPayloadsGroup{{Payloads: payloads}}
	return p
}

func TestValidateRejectsInvalidMode(t *testing.T) {
	err := Validate("garbage", []FuzzerPosition{mkPos(0, 1, "a")}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.Error(t, err)
}

func TestValidateRequiresPositions(t *testing.T) {
	err := Validate(ModeSingle, []FuzzerPosition{}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.ErrorContains(t, err, "at least one")
}

func TestValidateRejectsInvalidByteRange(t *testing.T) {
	err := Validate(ModeSingle, []FuzzerPosition{{Start: 10, End: 5}}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.ErrorContains(t, err, "end")
}

func TestValidateRejectsOverlap(t *testing.T) {
	positions := []FuzzerPosition{
		mkPos(0, 10, "first"),
		mkPos(5, 15, "second"),
	}
	err := Validate(ModeSingle, positions, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.ErrorContains(t, err, "overlap")
}

func TestValidateAllowsTouchingButNonOverlappingPositions(t *testing.T) {
	positions := []FuzzerPosition{
		mkPos(0, 5, "a"),
		mkPos(5, 10, "b"), // end of first == start of second, non-overlapping
	}
	err := Validate(ModeSingle, positions, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.NoError(t, err)
}

func TestValidateSingleRequiresShared(t *testing.T) {
	err := Validate(ModeSingle, []FuzzerPosition{mkPos(0, 1, "a")}, nil)
	require.ErrorContains(t, err, "shared_payloads")
}

func TestValidateSingleRejectsPositionGroups(t *testing.T) {
	pos := mkPosWithGroup(0, 1, "a", "p1")
	err := Validate(ModeSingle, []FuzzerPosition{pos}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.ErrorContains(t, err, "must not have payload_groups")
}

func TestValidateSingleRejectsEmptyShared(t *testing.T) {
	err := Validate(ModeSingle, []FuzzerPosition{mkPos(0, 1, "a")}, &FuzzerPayloadsGroup{Payloads: []string{}})
	require.ErrorContains(t, err, "empty")
}

func TestValidatePairedRejectsShared(t *testing.T) {
	err := Validate(ModePaired, []FuzzerPosition{mkPosWithGroup(0, 1, "a", "x")}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.ErrorContains(t, err, "must be nil")
}

func TestValidatePairedRequiresPositionGroups(t *testing.T) {
	err := Validate(ModePaired, []FuzzerPosition{mkPos(0, 1, "a")}, nil)
	require.ErrorContains(t, err, "payload_groups")
}

func TestValidateCombinationsRequiresPositionGroups(t *testing.T) {
	err := Validate(ModeCombinations, []FuzzerPosition{mkPos(0, 1, "a")}, nil)
	require.ErrorContains(t, err, "payload_groups")
}

func TestValidateAcceptsValidSingle(t *testing.T) {
	err := Validate(ModeSingle, []FuzzerPosition{mkPos(0, 1, "a")}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.NoError(t, err)
}

func TestValidateAcceptsValidAll(t *testing.T) {
	err := Validate(ModeAll, []FuzzerPosition{mkPos(0, 1, "a"), mkPos(2, 3, "b")}, &FuzzerPayloadsGroup{Payloads: []string{"x"}})
	require.NoError(t, err)
}

func TestValidateAcceptsValidPaired(t *testing.T) {
	positions := []FuzzerPosition{
		mkPosWithGroup(0, 1, "a", "p1", "p2"),
		mkPosWithGroup(2, 3, "b", "q1", "q2"),
	}
	err := Validate(ModePaired, positions, nil)
	require.NoError(t, err)
}

func TestValidateAcceptsValidCombinations(t *testing.T) {
	positions := []FuzzerPosition{
		mkPosWithGroup(0, 1, "a", "p1", "p2"),
		mkPosWithGroup(2, 3, "b", "q1", "q2", "q3"),
	}
	err := Validate(ModeCombinations, positions, nil)
	require.NoError(t, err)
}
