package clipsearch

import (
	"testing"
)

func TestTokenizeSimple_DifferentInputsDifferentTokens(t *testing.T) {
	dog := tokenizeSimple("dog")
	cat := tokenizeSimple("cat")

	if dog[1] == cat[1] {
		t.Errorf("tokenizeSimple(\"dog\") and tokenizeSimple(\"cat\") produced the same token at position 1: %d", dog[1])
	}
}

func TestTokenizeSimple_EmptyString(t *testing.T) {
	tokens := tokenizeSimple("")

	if tokens[0] != startToken {
		t.Errorf("position 0 = %d, want startToken %d", tokens[0], startToken)
	}
	if tokens[1] != endToken {
		t.Errorf("position 1 = %d, want endToken %d", tokens[1], endToken)
	}
	// All remaining positions must be zero.
	for i := 2; i < clipSeqLen; i++ {
		if tokens[i] != 0 {
			t.Errorf("position %d = %d, want 0 (padding)", i, tokens[i])
		}
	}
}

func TestTokenizeSimple_SequenceLength(t *testing.T) {
	tokens := tokenizeSimple("a red car near the gate")
	if len(tokens) != clipSeqLen {
		t.Fatalf("len = %d, want %d", len(tokens), clipSeqLen)
	}
}

func TestTokenizeSimple_Deterministic(t *testing.T) {
	a := tokenizeSimple("hello world")
	b := tokenizeSimple("hello world")

	for i := range a {
		if a[i] != b[i] {
			t.Errorf("non-deterministic at position %d: %d vs %d", i, a[i], b[i])
		}
	}
}

func TestTokenizeSimple_CaseInsensitive(t *testing.T) {
	lower := tokenizeSimple("dog")
	upper := tokenizeSimple("DOG")

	if lower[1] != upper[1] {
		t.Errorf("case sensitivity: \"dog\" token=%d, \"DOG\" token=%d", lower[1], upper[1])
	}
}

func TestTokenizeSimple_TokenRange(t *testing.T) {
	tokens := tokenizeSimple("the quick brown fox jumps over the lazy dog")
	for i := 1; i < clipSeqLen; i++ {
		if tokens[i] == endToken {
			break
		}
		if tokens[i] == 0 {
			break
		}
		if tokens[i] < 1 || tokens[i] > vocabSize {
			t.Errorf("token at position %d = %d, out of range [1, %d]", i, tokens[i], vocabSize)
		}
	}
}

func TestTokenizeSimple_MultiWordDiffersFromSingleWord(t *testing.T) {
	single := tokenizeSimple("dog")
	multi := tokenizeSimple("dog cat")

	// Position 2 should differ: single has endToken, multi has cat's token.
	if single[2] == multi[2] {
		t.Errorf("single-word and multi-word should differ at position 2: both = %d", single[2])
	}
}
