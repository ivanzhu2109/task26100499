package reuse

import (
	"fmt"
	"testing"
)

func TestVocabInternLookup(t *testing.T) {
	v := newVocab()
	const n = 100000 // 触发多次扩容
	for i := 0; i < n; i++ {
		w := fmt.Sprintf("w%d", i)
		if id := v.intern([]byte(w)); int(id) != i {
			t.Fatalf("intern(%q) = %d, want %d", w, id, i)
		}
	}
	if v.size() != n {
		t.Fatalf("size = %d, want %d", v.size(), n)
	}
	for i := 0; i < n; i++ {
		w := fmt.Sprintf("w%d", i)
		id, ok := v.lookup([]byte(w))
		if !ok || int(id) != i {
			t.Fatalf("lookup(%q) = %d,%v", w, id, ok)
		}
		if again := v.intern([]byte(w)); again != id {
			t.Fatalf("re-intern(%q) = %d, want %d", w, again, id)
		}
	}
	if _, ok := v.lookup([]byte("missing")); ok {
		t.Fatal("lookup(missing) should fail")
	}
}
