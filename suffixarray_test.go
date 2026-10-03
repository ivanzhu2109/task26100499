package reuse

import (
	"math/rand"
	"slices"
	"testing"
)

func checkSA(t *testing.T, ids []uint32) {
	t.Helper()
	sa := buildSuffixArray(ids)
	n := len(ids)
	if len(sa) != n {
		t.Fatalf("len(sa) = %d, want %d", len(sa), n)
	}
	seen := make([]bool, n)
	less := func(a, b int) bool {
		for a < n && b < n {
			if ids[a] != ids[b] {
				return ids[a] < ids[b]
			}
			a++
			b++
		}
		return a == n && b < n // 前缀更短者小
	}
	for i, s := range sa {
		if seen[s] {
			t.Fatalf("duplicate suffix %d", s)
		}
		seen[s] = true
		if i > 0 && !less(int(sa[i-1]), int(s)) && !slices.Equal(ids[sa[i-1]:], ids[s:]) {
			t.Fatalf("sa out of order at %d: %d %d", i, sa[i-1], s)
		}
	}
}

func TestSuffixArraySmall(t *testing.T) {
	checkSA(t, nil)
	checkSA(t, []uint32{0})
	checkSA(t, []uint32{1, 0, 1, 0, 1})
	checkSA(t, []uint32{3, 1, 2, 1, 3, 1, 2, 1})
	checkSA(t, []uint32{5, 5, 5, 5, 5, 5, 5})
}

func TestSuffixArrayRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 20; trial++ {
		n := 1 + rng.Intn(200)
		v := 1 + rng.Intn(5)
		ids := make([]uint32, n)
		for i := range ids {
			ids[i] = uint32(rng.Intn(v))
		}
		checkSA(t, ids)
	}
}
