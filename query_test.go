package reuse

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
)

// bruteEvidence 穷举所有待查起点与来源位置的标准实现，用于对照。
func bruteEvidence(srcs []Source, query string) []Evidence {
	type doc struct {
		id    uint32
		words [][]byte
		spans []wordSpan
	}
	var docs []doc
	for _, s := range srcs {
		spans, words := tokenizeLower(s.Text)
		docs = append(docs, doc{s.ID, words, spans})
	}
	qSpans, qWords := tokenizeLower(query)
	m := len(qWords)

	type cand struct {
		start, length int
		srcID         uint32
		srcPos        int
	}
	var cands []cand
	for i := 0; i < m; i++ {
		best := 0
		var bSrc uint32
		var bPos int
		found := false
		for _, d := range docs {
			for p := 0; p < len(d.words); p++ {
				l := 0
				for i+l < m && p+l < len(d.words) &&
					string(qWords[i+l]) == string(d.words[p+l]) {
					l++
				}
				if !found || l > best || (l == best && l > 0 &&
					(d.id < bSrc || (d.id == bSrc && p < bPos))) {
					if l > 0 {
						found = true
						best, bSrc, bPos = l, d.id, p
					}
				}
			}
		}
		if found && best >= MinEvidenceWords {
			cands = append(cands, cand{i, best, bSrc, bPos})
		}
	}

	var out []Evidence
	for _, c1 := range cands {
		contained := false
		for _, c2 := range cands {
			if c2.start <= c1.start &&
				c2.start+c2.length >= c1.start+c1.length &&
				(c2.start < c1.start || c2.start+c2.length > c1.start+c1.length) {
				contained = true
				break
			}
		}
		if contained {
			continue
		}
		var d *doc
		for k := range docs {
			if docs[k].id == c1.srcID {
				d = &docs[k]
				break
			}
		}
		out = append(out, Evidence{
			SourceID:    c1.srcID,
			QueryStart:  qSpans[c1.start].start,
			QueryEnd:    qSpans[c1.start+c1.length-1].end,
			SourceStart: d.spans[c1.srcPos].start,
			SourceEnd:   d.spans[c1.srcPos+c1.length-1].end,
		})
	}
	return out
}

func runesSlice(s string, start, end int) string {
	rs := []rune(s)
	return string(rs[start:end])
}

func buildAndQuery(t *testing.T, srcs []Source, query string) []Evidence {
	t.Helper()
	dir := t.TempDir()
	if err := Build(dir, srcs); err != nil {
		t.Fatal(err)
	}
	c, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.Query(query)
}

func checkAgainstBrute(t *testing.T, srcs []Source, query string) {
	t.Helper()
	got := buildAndQuery(t, srcs, query)
	want := bruteEvidence(srcs, query)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query %q\n got: %+v\nwant: %+v", query, got, want)
	}
}

func TestDirectedTieSources(t *testing.T) {
	text := "one two three four five six seven eight nine"
	srcs := []Source{{ID: 5, Text: text}, {ID: 2, Text: text}}
	got := buildAndQuery(t, srcs, text)
	if len(got) != 1 || got[0].SourceID != 2 {
		t.Fatalf("want single evidence from source 2, got %+v", got)
	}
	checkAgainstBrute(t, srcs, text)
}

func TestDirectedEarliestOccurrence(t *testing.T) {
	srcs := []Source{{ID: 1, Text: "a b c d e f g h z z a b c d e f g h"}}
	q := "a b c d e f g h"
	got := buildAndQuery(t, srcs, q)
	if len(got) != 1 || got[0].SourceStart != 0 {
		t.Fatalf("want earliest occurrence at 0, got %+v", got)
	}
	checkAgainstBrute(t, srcs, q)
}

func TestDirectedDocBoundary(t *testing.T) {
	// 8 词横跨两份文档交界处，不能拼成证据。
	srcs := []Source{
		{ID: 1, Text: "w1 w2 w3 w4"},
		{ID: 2, Text: "w5 w6 w7 w8"},
	}
	got := buildAndQuery(t, srcs, "w1 w2 w3 w4 w5 w6 w7 w8")
	if len(got) != 0 {
		t.Fatalf("cross-document match must not form evidence, got %+v", got)
	}
	checkAgainstBrute(t, srcs, "w1 w2 w3 w4 w5 w6 w7 w8")
}

func TestDirectedMinLength(t *testing.T) {
	srcs := []Source{{ID: 1, Text: "a b c d e f g h"}}
	if got := buildAndQuery(t, srcs, "a b c d e f g"); len(got) != 0 {
		t.Fatalf("7 words must not form evidence, got %+v", got)
	}
	if got := buildAndQuery(t, srcs, "a b c d e f g h"); len(got) != 1 {
		t.Fatalf("8 words must form evidence, got %+v", got)
	}
}

func TestDirectedOverlapKept(t *testing.T) {
	// 两段证据相互交叠但互不包含，必须分别保留。
	srcs := []Source{
		{ID: 1, Text: "a b c d e f g h"},
		{ID: 2, Text: "e f g h i j k l"},
	}
	got := buildAndQuery(t, srcs, "a b c d e f g h i j k l")
	if len(got) != 2 || got[0].SourceID != 1 || got[1].SourceID != 2 {
		t.Fatalf("overlapping evidence must be kept separately, got %+v", got)
	}
	checkAgainstBrute(t, srcs, "a b c d e f g h i j k l")
}

func TestDirectedNesting(t *testing.T) {
	srcs := []Source{{ID: 1, Text: "sa sb sc sd se sf sg sh si sj"}}
	got := buildAndQuery(t, srcs, "sa sb sc sd se sf sg sh si sj")
	if len(got) != 1 || got[0].QueryStart != 0 || got[0].QueryEnd != 29 {
		t.Fatalf("nested evidence must collapse to the outermost, got %+v", got)
	}
}

func TestDirectedCaseInsensitive(t *testing.T) {
	srcs := []Source{{ID: 1, Text: "Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel"}}
	got := buildAndQuery(t, srcs, "alpha bravo charlie delta echo foxtrot golf hotel")
	if len(got) != 1 {
		t.Fatalf("case-insensitive match expected, got %+v", got)
	}
}

func TestDirectedMultibyteSeparators(t *testing.T) {
	srcText := "alpha——beta，gamma；delta\techo\nfoxtrot　golf hotel"
	qText := "alpha beta gamma delta echo foxtrot golf hotel"
	srcs := []Source{{ID: 1, Text: srcText}}
	got := buildAndQuery(t, srcs, qText)
	if len(got) != 1 {
		t.Fatalf("expected one evidence, got %+v", got)
	}
	e := got[0]
	if s := runesSlice(srcText, e.SourceStart, e.SourceEnd); s != srcText {
		t.Fatalf("source slice = %q, want %q", s, srcText)
	}
	if s := runesSlice(qText, e.QueryStart, e.QueryEnd); s != qText {
		t.Fatalf("query slice = %q, want %q", s, qText)
	}
	// 两侧多字节分隔符不应进入区间。
	srcs2 := []Source{{ID: 1, Text: "「alpha beta gamma delta echo foxtrot golf hotel」"}}
	got2 := buildAndQuery(t, srcs2, "「alpha beta gamma delta echo foxtrot golf hotel」")
	if len(got2) != 1 {
		t.Fatalf("expected one evidence, got %+v", got2)
	}
	e2 := got2[0]
	if s := runesSlice(srcs2[0].Text, e2.SourceStart, e2.SourceEnd); s != "alpha beta gamma delta echo foxtrot golf hotel" {
		t.Fatalf("source slice = %q", s)
	}
	checkAgainstBrute(t, srcs2, "「alpha beta gamma delta echo foxtrot golf hotel」")
}

func TestRandomizedAgainstBrute(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	vocab := []string{"a", "bb", "ccc", "dd", "e", "ffff", "g", "hh"}
	seps := []string{" ", ", ", "——", "。", "  ", "\n", "、", "-"}
	randText := func(n int) string {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteString(seps[rng.Intn(len(seps))])
			}
			w := vocab[rng.Intn(len(vocab))]
			if rng.Intn(4) == 0 {
				w = strings.ToUpper(w)
			}
			sb.WriteString(w)
		}
		return sb.String()
	}
	totalEvidence := 0
	for trial := 0; trial < 200; trial++ {
		var srcs []Source
		nSrc := 1 + rng.Intn(5)
		for s := 0; s < nSrc; s++ {
			srcs = append(srcs, Source{ID: uint32(1 + rng.Intn(50)), Text: randText(rng.Intn(30))})
		}
		// 去重编号
		seen := map[uint32]bool{}
		uniq := srcs[:0]
		for _, s := range srcs {
			if !seen[s.ID] {
				seen[s.ID] = true
				uniq = append(uniq, s)
			}
		}
		srcs = uniq
		// 查询：一半随机，一半拼接来源片段以制造长命中
		var query string
		if rng.Intn(2) == 0 || len(srcs) == 0 {
			query = randText(rng.Intn(40))
		} else {
			query = srcs[rng.Intn(len(srcs))].Text + " " + randText(rng.Intn(10))
		}
		checkAgainstBrute(t, srcs, query)
		totalEvidence += len(bruteEvidence(srcs, query))
	}
	if totalEvidence == 0 {
		t.Fatal("randomized trials produced no evidence; generator too weak")
	}
}

func TestRandomizedPeriodic(t *testing.T) {
	// 周期性重复语料：同一前缀出现极多次。
	rng := rand.New(rand.NewSource(11))
	for trial := 0; trial < 30; trial++ {
		period := 1 + rng.Intn(4)
		var sb strings.Builder
		words := []string{"a", "b", "c", "d"}
		for i := 0; i < 60; i++ {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(words[i%period])
		}
		srcs := []Source{{ID: 1, Text: sb.String()}, {ID: 2, Text: sb.String()}}
		var qb strings.Builder
		for i := 0; i < 40; i++ {
			if i > 0 {
				qb.WriteString(" ")
			}
			qb.WriteString(words[(i+1)%period])
		}
		checkAgainstBrute(t, srcs, qb.String())
	}
}

func TestEmptyInputs(t *testing.T) {
	if got := buildAndQuery(t, nil, "a b c d e f g h"); len(got) != 0 {
		t.Fatalf("empty corpus: %+v", got)
	}
	srcs := []Source{{ID: 1, Text: "a b c d e f g h"}}
	if got := buildAndQuery(t, srcs, ""); len(got) != 0 {
		t.Fatalf("empty query: %+v", got)
	}
	if got := buildAndQuery(t, srcs, "x y z"); len(got) != 0 {
		t.Fatalf("short query: %+v", got)
	}
	if got := buildAndQuery(t, srcs, "q1 q2 q3 q4 q5 q6 q7 q8"); len(got) != 0 {
		t.Fatalf("unknown words: %+v", got)
	}
}

func TestDuplicateIDRejected(t *testing.T) {
	dir := t.TempDir()
	err := Build(dir, []Source{{ID: 1, Text: "a"}, {ID: 1, Text: "b"}})
	if err == nil {
		t.Fatal("duplicate IDs must be rejected")
	}
	if err := Build(dir, []Source{{ID: 0, Text: "a"}}); err == nil {
		t.Fatal("zero ID must be rejected")
	}
}

func TestEvidenceCoordinatesSlice(t *testing.T) {
	srcs := []Source{
		{ID: 3, Text: "prefix the quick brown fox jumps over the lazy dog suffix"},
		{ID: 1, Text: "unrelated text here"},
	}
	q := "well the quick brown fox jumps over the lazy dog indeed"
	got := buildAndQuery(t, srcs, q)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	e := got[0]
	want := "the quick brown fox jumps over the lazy dog"
	if s := runesSlice(srcs[0].Text, e.SourceStart, e.SourceEnd); s != want {
		t.Fatalf("source slice %q want %q", s, want)
	}
	if s := runesSlice(q, e.QueryStart, e.QueryEnd); s != want {
		t.Fatalf("query slice %q want %q", s, want)
	}
	if e.SourceID != 3 {
		t.Fatalf("source id = %d", e.SourceID)
	}
}

func ExampleCorpus_Query() {
	dir, err := os.MkdirTemp("", "reuse-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	srcs := []Source{{ID: 1, Text: "the quick brown fox jumps over the lazy dog"}}
	if err := Build(dir, srcs); err != nil {
		panic(err)
	}
	c, _ := Open(dir)
	defer c.Close()
	for _, e := range c.Query("he said the quick brown fox jumps over the lazy dog again") {
		fmt.Println(e.SourceID, e.QueryStart, e.QueryEnd, e.SourceStart, e.SourceEnd)
	}
	// Output: 1 8 51 0 43
}

func TestDirectedLengthBeatsEarlierShortMatch(t *testing.T) {
	// 来源 1 含 8 词匹配，来源 2 含 10 词匹配：先比长度，必须选来源 2。
	srcs := []Source{
		{ID: 1, Text: "a b c d e f g h"},
		{ID: 2, Text: "a b c d e f g h i j"},
	}
	got := buildAndQuery(t, srcs, "a b c d e f g h i j")
	if len(got) != 1 || got[0].SourceID != 2 {
		t.Fatalf("longer match must win over smaller source id, got %+v", got)
	}
	checkAgainstBrute(t, srcs, "a b c d e f g h i j")
}

func TestDirectedSameSourceEarliestPosition(t *testing.T) {
	// 同一来源中同一片段出现两次，取起始位置最早的一次。
	srcs := []Source{{ID: 7, Text: "x x a b c d e f g h y y a b c d e f g h"}}
	got := buildAndQuery(t, srcs, "a b c d e f g h")
	if len(got) != 1 || got[0].SourceStart != 4 {
		t.Fatalf("want earliest occurrence (rune 4), got %+v", got)
	}
}

func TestDirectedUnknownWordBreaksMatch(t *testing.T) {
	srcs := []Source{{ID: 1, Text: "a b c d e f g h i j k l"}}
	// 中间出现语料没有的词，长链断成两段 8 词证据。
	got := buildAndQuery(t, srcs, "a b c d e f g h zz i j k l")
	if len(got) != 1 || got[0].QueryStart != 0 || got[0].QueryEnd != 15 {
		t.Fatalf("only the first 8 words form evidence, got %+v", got)
	}
	got = buildAndQuery(t, srcs, "a b c d e f g h h i j k l m n o p")
	if len(got) != 1 || got[0].QueryStart != 0 || got[0].QueryEnd != 15 {
		t.Fatalf("only the first 8 words form evidence, got %+v", got)
	}
	checkAgainstBrute(t, srcs, "a b c d e f g h h i j k l m n o p")
}

func TestDirectedOverlapAcrossSources(t *testing.T) {
	// 两段证据区间交叠但互不包含：分别保留，不得合并。
	srcs := []Source{
		{ID: 1, Text: "a b c d e f g h i j"},
		{ID: 2, Text: "f g h i j k l m n o"},
	}
	q := "a b c d e f g h i j k l m n o"
	got := buildAndQuery(t, srcs, q)
	if len(got) != 2 || got[0].SourceID != 1 || got[1].SourceID != 2 {
		t.Fatalf("overlapping non-containing evidence must both survive, got %+v", got)
	}
	checkAgainstBrute(t, srcs, q)
}
