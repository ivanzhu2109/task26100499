package reuse

import (
	"bytes"
	"testing"
)

func TestVocabBuildAndLookup(t *testing.T) {
	words := [][]byte{[]byte("banana"), []byte("apple"), []byte("banana"), []byte("cherry")}
	var blob []byte
	off := []uint32{0}
	for _, w := range words {
		blob = append(blob, w...)
		off = append(off, uint32(len(blob)))
	}
	v, ids := buildVocab(blob, off)
	if v.size() != 3 {
		t.Fatalf("size = %d", v.size())
	}
	if ids[0] != ids[2] {
		t.Fatalf("duplicate word got different ids: %v", ids)
	}
	if ids[0] == ids[1] || ids[1] == ids[3] || ids[0] == ids[3] {
		t.Fatalf("distinct words share id: %v", ids)
	}
	for i, w := range words {
		if got := v.id(w); got != ids[i] {
			t.Fatalf("id(%q) = %d, want %d", w, got, ids[i])
		}
	}
	if got := v.id([]byte("missing")); got != 0 {
		t.Fatalf("missing word id = %d", got)
	}
}

func TestVocabPersistence(t *testing.T) {
	words := [][]byte{[]byte("b"), []byte("a"), []byte("a"), []byte("xyz")}
	var blob []byte
	off := []uint32{0}
	for _, w := range words {
		blob = append(blob, w...)
		off = append(off, uint32(len(blob)))
	}
	v, _ := buildVocab(blob, off)
	var buf bytes.Buffer
	if err := v.writeTo(&buf); err != nil {
		t.Fatal(err)
	}
	v2, err := readVocab(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if v2.size() != v.size() {
		t.Fatalf("size %d != %d", v2.size(), v.size())
	}
	for _, w := range []string{"a", "b", "xyz"} {
		if v2.id([]byte(w)) != v.id([]byte(w)) {
			t.Fatalf("id mismatch for %q", w)
		}
	}
}
