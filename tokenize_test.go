package reuse

import (
	"reflect"
	"testing"
)

func TestTokenizeBasic(t *testing.T) {
	spans := tokenize("Hello, World!")
	want := []wordSpan{{0, 5}, {7, 12}}
	if !reflect.DeepEqual(spans, want) {
		t.Fatalf("got %v want %v", spans, want)
	}
}

func TestTokenizeMultibyteSeparators(t *testing.T) {
	// “ab”2 码点，中文逗号 1 码点，“cd”从码点 3 开始。
	spans := tokenize("ab，cd")
	want := []wordSpan{{0, 2}, {3, 5}}
	if !reflect.DeepEqual(spans, want) {
		t.Fatalf("got %v want %v", spans, want)
	}
}

func TestTokenizeLowerWords(t *testing.T) {
	spans, words := tokenizeLower("AbC 一xyZ")
	if len(spans) != 2 || len(words) != 2 {
		t.Fatalf("got %v %v", spans, words)
	}
	if string(words[0]) != "abc" || string(words[1]) != "xyz" {
		t.Fatalf("words: %q %q", words[0], words[1])
	}
}

func TestTokenizeEdge(t *testing.T) {
	if got := tokenize(""); got != nil {
		t.Fatalf("empty: %v", got)
	}
	if got := tokenize("123 --"); got != nil {
		t.Fatalf("no words: %v", got)
	}
	spans := tokenize("tail")
	want := []wordSpan{{0, 4}}
	if !reflect.DeepEqual(spans, want) {
		t.Fatalf("got %v want %v", spans, want)
	}
}
