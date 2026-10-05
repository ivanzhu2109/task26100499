package reuse

import (
	"reflect"
	"testing"
)

func TestTokenizeBasic(t *testing.T) {
	words, norms := Tokenize("Hello, WORLD! foo-bar")
	wantWords := []Word{{0, 5}, {7, 12}, {14, 17}, {18, 21}}
	wantNorms := []string{"hello", "world", "foo", "bar"}
	if !reflect.DeepEqual(words, wantWords) {
		t.Fatalf("words = %v, want %v", words, wantWords)
	}
	if !reflect.DeepEqual(norms, wantNorms) {
		t.Fatalf("norms = %v, want %v", norms, wantNorms)
	}
}

func TestTokenizeMultibyteSeparators(t *testing.T) {
	// “你好”为 2 个码点；坐标应按码点计数。
	words, norms := Tokenize("ab你好cd")
	wantWords := []Word{{0, 2}, {4, 6}}
	wantNorms := []string{"ab", "cd"}
	if !reflect.DeepEqual(words, wantWords) {
		t.Fatalf("words = %v, want %v", words, wantWords)
	}
	if !reflect.DeepEqual(norms, wantNorms) {
		t.Fatalf("norms = %v, want %v", norms, wantNorms)
	}
}

func TestTokenizeEmptyAndNoLetters(t *testing.T) {
	for _, s := range []string{"", "123 !@# 你好"} {
		if w, n := Tokenize(s); len(w) != 0 || len(n) != 0 {
			t.Fatalf("Tokenize(%q) = %v, %v; want empty", s, w, n)
		}
	}
}
