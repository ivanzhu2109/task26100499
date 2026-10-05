package reuse

import (
	"math/rand"
	"sort"
	"testing"
)

func naiveSuffixArray(tokens []uint32) []int32 {
	sa := make([]int32, len(tokens))
	for i := range sa {
		sa[i] = int32(i)
	}
	sort.Slice(sa, func(x, y int) bool {
		a, b := int(sa[x]), int(sa[y])
		for a < len(tokens) && b < len(tokens) {
			if tokens[a] != tokens[b] {
				return tokens[a] < tokens[b]
			}
			a++
			b++
		}
		if a == len(tokens) && b == len(tokens) {
			return false
		}
		return a == len(tokens)
	})
	return sa
}

func checkSA(t *testing.T, tokens []uint32) {
	t.Helper()
	got := buildSuffixArray(tokens)
	want := naiveSuffixArray(tokens)
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens=%v sa[%d] = %d, want %d", tokens, i, got[i], want[i])
		}
	}
}

func TestSuffixArraySmall(t *testing.T) {
	checkSA(t, nil)
	checkSA(t, []uint32{0})
	checkSA(t, []uint32{2, 1, 2, 1, 3})
	checkSA(t, []uint32{0, 0, 0, 0, 0})
	checkSA(t, []uint32{5, 4, 3, 2, 1, 0})
}

func TestSuffixArrayRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 50; trial++ {
		n := 1 + rng.Intn(200)
		tokens := make([]uint32, n)
		for i := range tokens {
			tokens[i] = uint32(rng.Intn(7))
		}
		checkSA(t, tokens)
	}
}

func TestSuffixArrayPeriodic(t *testing.T) {
	// 长周期重复：前缀倍增必须正确处理相同长前缀。
	tokens := make([]uint32, 0, 4000)
	for i := 0; i < 1000; i++ {
		tokens = append(tokens, 1, 2, 3, 4)
	}
	got := buildSuffixArray(tokens)
	// 校验有序性而非逐项对比（朴素法 O(n^2) 太慢）。
	for i := 1; i < len(got); i++ {
		a, b := int(got[i-1]), int(got[i])
		less := false
		for a < len(tokens) && b < len(tokens) {
			if tokens[a] != tokens[b] {
				less = tokens[a] < tokens[b]
				break
			}
			a++
			b++
		}
		if a == len(tokens) && b < len(tokens) {
			less = true
		}
		if !less {
			t.Fatalf("sa not sorted at %d: %d vs %d", i, got[i-1], got[i])
		}
	}
}
