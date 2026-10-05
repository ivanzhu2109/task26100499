package reuse

import (
	"math/rand"
	"testing"
)

func TestRangeMin(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for trial := 0; trial < 100; trial++ {
		n := 1 + rng.Intn(300)
		keys := make([]uint64, n)
		for i := range keys {
			keys[i] = uint64(rng.Intn(1000))<<32 | uint64(i)
		}
		rm := newRangeMin(keys)
		for q := 0; q < 50; q++ {
			lo := rng.Intn(n)
			hi := lo + rng.Intn(n-lo+1)
			got := rm.argMin(lo, hi)
			want := -1
			for i := lo; i < hi; i++ {
				if want < 0 || keys[i] < keys[want] {
					want = i
				}
			}
			if got != want {
				t.Fatalf("argMin(%d,%d) = %d, want %d", lo, hi, got, want)
			}
		}
	}
	if rm := newRangeMin([]uint64{5}); rm.argMin(0, 0) != -1 {
		t.Fatal("empty range should return -1")
	}
}
