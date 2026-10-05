package reuse

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// bruteForce 穷举所有待查起点与来源位置，作为正确性标准。
func bruteForce(t *testing.T, ids []int, docs []string, query string) []Evidence {
	t.Helper()
	qWords, qNorms := Tokenize(query)
	type docInfo struct {
		id    int
		words []Word
		norms []string
	}
	ds := make([]docInfo, len(docs))
	for i := range docs {
		w, n := Tokenize(docs[i])
		ds[i] = docInfo{id: ids[i], words: w, norms: n}
	}
	type cand struct {
		qWord, length, di, pos int
	}
	var cands []cand
	for qi := 0; qi+MinMatchWords <= len(qNorms); qi++ {
		best, bdi, bpos := 0, -1, 0
		for di := range ds {
			for p := 0; p < len(ds[di].norms); p++ {
				l := 0
				for qi+l < len(qNorms) && p+l < len(ds[di].norms) && qNorms[qi+l] == ds[di].norms[p+l] {
					l++
				}
				if l > best ||
					(l == best && l > 0 && bdi >= 0 && (ds[di].id < ds[bdi].id ||
						(ds[di].id == ds[bdi].id && p < bpos))) ||
					(l == best && l > 0 && bdi < 0) {
					best, bdi, bpos = l, di, p
				}
			}
		}
		if best >= MinMatchWords {
			cands = append(cands, cand{qi, best, bdi, bpos})
		}
	}
	var ev []Evidence
	maxEnd := 0
	for _, c := range cands {
		end := c.qWord + c.length
		if end <= maxEnd {
			continue
		}
		maxEnd = end
		d := ds[c.di]
		ev = append(ev, Evidence{
			DocID:       d.id,
			QueryStart:  qWords[c.qWord].Start,
			QueryEnd:    qWords[end-1].End,
			SourceStart: d.words[c.pos].Start,
			SourceEnd:   d.words[c.pos+c.length-1].End,
		})
	}
	return ev
}

func buildIndex(t *testing.T, ids []int, docs []string) *Index {
	t.Helper()
	b := NewBuilder()
	for i := range docs {
		if err := b.AddDocument(ids[i], docs[i]); err != nil {
			t.Fatalf("AddDocument: %v", err)
		}
	}
	idx, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return idx
}

func checkQuery(t *testing.T, ids []int, docs []string, query string) {
	t.Helper()
	idx := buildIndex(t, ids, docs)
	got := idx.Query(query)
	want := bruteForce(t, ids, docs, query)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query %q\n got: %+v\nwant: %+v", query, got, want)
	}
}

func TestQueryRandomCrossCheck(t *testing.T) {
	crossCheck(t, 42, []string{"the", "a", "of", "x", "y", "z", "repeat", "common"}, 200)
}

// 词表极小，重复词、并列来源、嵌套与交叠高频出现。
func TestQueryRandomCrossCheckDense(t *testing.T) {
	crossCheck(t, 7, []string{"a", "b", "c"}, 500)
}

func crossCheck(t *testing.T, seed int64, vocab []string, trials int) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	seps := []string{" ", " ", ", ", ". ", "，", "你好", "\n", "!!", " ' "}
	render := func(words []string) string {
		var sb strings.Builder
		for i, w := range words {
			if i > 0 {
				sb.WriteString(seps[rng.Intn(len(seps))])
			}
			if rng.Intn(2) == 0 {
				w = strings.ToUpper(w)
			}
			sb.WriteString(w)
		}
		return sb.String()
	}
	randWords := func(n int) []string {
		ws := make([]string, n)
		for i := range ws {
			ws[i] = vocab[rng.Intn(len(vocab))]
		}
		return ws
	}
	for trial := 0; trial < trials; trial++ {
		nDocs := 1 + rng.Intn(4)
		ids := rng.Perm(100)[:nDocs]
		for i := range ids {
			ids[i]++
		}
		docWords := make([][]string, nDocs)
		for i := range docWords {
			docWords[i] = randWords(rng.Intn(30))
		}
		// 查询由随机噪声与来源词序列片段拼接而成，保证命中、嵌套与交叠。
		var qWords []string
		for len(qWords) < 40 {
			switch rng.Intn(3) {
			case 0:
				qWords = append(qWords, randWords(1+rng.Intn(5))...)
			default:
				d := docWords[rng.Intn(nDocs)]
				if len(d) == 0 {
					continue
				}
				s := rng.Intn(len(d))
				e := s + rng.Intn(len(d)-s) + 1
				qWords = append(qWords, d[s:e]...)
			}
		}
		docs := make([]string, nDocs)
		for i := range docs {
			docs[i] = render(docWords[i])
		}
		query := render(qWords)
		checkQuery(t, ids, docs, query)
	}
}

func TestQueryRepeatedWords(t *testing.T) {
	// 重复词不得被跳过：10 个相同词应形成一段证据。
	idx := buildIndex(t, []int{1}, []string{"go go go go go go go go go go"})
	ev := idx.Query("go go go go go go go go")
	want := []Evidence{{DocID: 1, QueryStart: 0, QueryEnd: 23, SourceStart: 0, SourceEnd: 23}}
	if !reflect.DeepEqual(ev, want) {
		t.Fatalf("got %+v, want %+v", ev, want)
	}
}

func TestQueryTieBreakSmallestID(t *testing.T) {
	// 同一最长片段命中多份来源，选编号最小者。
	phrase := "w1 w2 w3 w4 w5 w6 w7 w8"
	idx := buildIndex(t, []int{7, 3, 9}, []string{phrase, phrase, phrase})
	ev := idx.Query(phrase)
	if len(ev) != 1 || ev[0].DocID != 3 {
		t.Fatalf("got %+v, want DocID 3", ev)
	}
}

func TestQueryTieBreakEarliestPos(t *testing.T) {
	// 同一来源中出现多次，选起始最早的一次。
	idx := buildIndex(t, []int{1}, []string{"a b c d e f g h x a b c d e f g h"})
	ev := idx.Query("a b c d e f g h")
	if len(ev) != 1 || ev[0].SourceStart != 0 {
		t.Fatalf("got %+v, want SourceStart 0", ev)
	}
}

func TestQueryLongerBeatsEarlierShorter(t *testing.T) {
	// 先比长度：编号更大但匹配更长的来源胜出。
	idx := buildIndex(t,
		[]int{1, 2},
		[]string{
			"a b c d e f g h",
			"a b c d e f g h i j",
		})
	ev := idx.Query("a b c d e f g h i j")
	if len(ev) != 1 || ev[0].DocID != 2 || ev[0].QueryEnd-ev[0].QueryStart != len("a b c d e f g h i j") {
		t.Fatalf("got %+v, want single evidence from doc 2", ev)
	}
}

func TestQueryNestedEvidenceDropped(t *testing.T) {
	// 被严格包含的候选不返回。
	idx := buildIndex(t, []int{1, 2}, []string{
		"a b c d e f g h i j k l",
		"c d e f g h i j",
	})
	ev := idx.Query("a b c d e f g h i j k l")
	if len(ev) != 1 || ev[0].DocID != 1 {
		t.Fatalf("got %+v, want only outer evidence from doc 1", ev)
	}
}

func TestQueryOverlappingKeptSeparate(t *testing.T) {
	// 相互交叠但互不包含的证据必须分别保留。
	idx := buildIndex(t, []int{1, 2}, []string{
		"a b c d e f g h i j",
		"f g h i j k l m n o",
	})
	ev := idx.Query("a b c d e f g h i j k l m n o")
	if len(ev) != 2 || ev[0].DocID != 1 || ev[1].DocID != 2 {
		t.Fatalf("got %+v, want two overlapping evidences", ev)
	}
}

func TestQueryNoCrossDocumentMatch(t *testing.T) {
	// 不同文档末尾与开头不能拼成 8 词证据。
	idx := buildIndex(t, []int{1, 2}, []string{
		"a b c d",
		"e f g h",
	})
	if ev := idx.Query("a b c d e f g h"); len(ev) != 0 {
		t.Fatalf("got %+v, want no evidence across document boundary", ev)
	}
}

func TestQueryMinLength(t *testing.T) {
	idx := buildIndex(t, []int{1}, []string{"a b c d e f g h"})
	if ev := idx.Query("a b c d e f g"); len(ev) != 0 {
		t.Fatalf("7 words: got %+v, want none", ev)
	}
	if ev := idx.Query("a b c d e f g h"); len(ev) != 1 {
		t.Fatalf("8 words: got %+v, want one", ev)
	}
}

func TestQueryMultibyteSeparators(t *testing.T) {
	// 多字节分隔符两侧的码点坐标必须能截取正确原文。
	src := "alpha，你好beta gamma、delta epsilon　zeta eta theta iota"
	query := "看beta gamma、delta epsilon　zeta eta theta iota！"
	idx := buildIndex(t, []int{5}, []string{src})
	ev := idx.Query(query)
	if len(ev) != 1 {
		t.Fatalf("got %+v, want one evidence", ev)
	}
	gotQ := []rune(query)[ev[0].QueryStart:ev[0].QueryEnd]
	gotS := []rune(src)[ev[0].SourceStart:ev[0].SourceEnd]
	if string(gotQ) != "beta gamma、delta epsilon　zeta eta theta iota" {
		t.Fatalf("query slice = %q", gotQ)
	}
	if string(gotS) != "beta gamma、delta epsilon　zeta eta theta iota" {
		t.Fatalf("source slice = %q", gotS)
	}
	if ev[0].DocID != 5 {
		t.Fatalf("DocID = %d, want 5", ev[0].DocID)
	}
}

func TestQueryCaseInsensitive(t *testing.T) {
	idx := buildIndex(t, []int{1}, []string{"Alpha BETA gamma DELTA epsilon ZETA eta THETA"})
	if ev := idx.Query("alpha beta Gamma delta EPSILON zeta Eta theta"); len(ev) != 1 {
		t.Fatalf("got %+v, want one evidence", ev)
	}
}

func TestQueryUnknownWords(t *testing.T) {
	// 语料中不存在的词不得产生命中，也不应干扰其余部分。
	idx := buildIndex(t, []int{1}, []string{"a b c d e f g h"})
	if ev := idx.Query("zz zz a b c d e f g h zz"); len(ev) != 1 {
		t.Fatalf("got %+v, want one evidence", ev)
	}
	if ev := idx.Query("zz1 zz2 zz3 zz4 zz5 zz6 zz7 zz8"); len(ev) != 0 {
		t.Fatalf("got %+v, want none", ev)
	}
}

func TestBuilderValidation(t *testing.T) {
	b := NewBuilder()
	if err := b.AddDocument(0, "x"); err == nil {
		t.Fatal("id 0 should be rejected")
	}
	if err := b.AddDocument(-3, "x"); err == nil {
		t.Fatal("negative id should be rejected")
	}
	if err := b.AddDocument(4, "x"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddDocument(4, "y"); err == nil {
		t.Fatal("duplicate id should be rejected")
	}
	if _, err := NewBuilder().Build(); err == nil {
		t.Fatal("empty corpus should be rejected")
	}
}

func TestQuerySortedOutput(t *testing.T) {
	// 证据按待查起点升序。
	rng := rand.New(rand.NewSource(7))
	ids := []int{1, 2, 3}
	docs := []string{
		"a b c d e f g h",
		"j k l m n o p q",
		"r s t u v w x y",
	}
	_ = rng
	idx := buildIndex(t, ids, docs)
	ev := idx.Query("r s t u v w x y zz a b c d e f g h")
	if len(ev) != 2 || ev[0].DocID != 3 || ev[1].DocID != 1 {
		t.Fatalf("got %+v", ev)
	}
	if !sort.SliceIsSorted(ev, func(i, j int) bool { return ev[i].QueryStart < ev[j].QueryStart }) {
		t.Fatal("evidence not sorted by query start")
	}
}

func Example() {
	b := NewBuilder()
	_ = b.AddDocument(1, "the quick brown fox jumps over the lazy dog")
	idx, _ := b.Build()
	for _, e := range idx.Query("a quick brown fox jumps over the lazy dog") {
		fmt.Printf("doc=%d query=[%d,%d) source=[%d,%d)\n",
			e.DocID, e.QueryStart, e.QueryEnd, e.SourceStart, e.SourceEnd)
	}
	// Output: doc=1 query=[2,41) source=[4,43)
}
