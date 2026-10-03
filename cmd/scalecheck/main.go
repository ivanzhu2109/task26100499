// scalecheck 是可复现的规模验证入口：
//
// 构造 2 万份来源、合计约 500 万词的语料，其中包含出现百万次的
// 公共 8 词前缀与长段周期性重复；随后执行 1 万词与 10 万词查询，
// 校验证据内容并报告结果数量、耗时与峰值内存。
//
// 用法：
//
//	go run ./cmd/scalecheck [-quick] [-keep] [-dir DIR]
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"reuse"
)

const mib = 1024 * 1024

// alphaEncode 生成纯字母编号（避免数字成为分隔符）。
func alphaEncode(i int) string {
	var buf [8]byte
	pos := len(buf)
	for {
		pos--
		buf[pos] = byte('a' + i%26)
		i /= 26
		if i == 0 {
			break
		}
	}
	return string(buf[pos:])
}

func patternWord(i int) string { return "pat" + alphaEncode(i) }
func fillerWord(i int) string  { return "fill" + alphaEncode(i) }

type corpusSpec struct {
	sharedRun   int // “shared” 单词重复次数（公共前缀来源）
	patternLen  int // 周期模式的词数
	patternReps int // 周期重复次数
	fillerDocs  int // 随机填充文档数
	fillerWords int // 填充总词数
	vocab       int // 填充词表大小
}

func buildCorpus(spec corpusSpec, seed int64) ([]reuse.Source, string) {
	rng := rand.New(rand.NewSource(seed))
	var srcs []reuse.Source

	// 文档 1：同一 8 词前缀出现百万次（sharedRun-7 次）。
	sharedText := strings.Repeat("shared ", spec.sharedRun)
	srcs = append(srcs, reuse.Source{ID: 1, Text: sharedText})

	// 文档 2：长段周期性重复。
	pattern := make([]string, spec.patternLen)
	for i := range pattern {
		pattern[i] = patternWord(i)
	}
	period := strings.Join(pattern, " ")
	var sb strings.Builder
	for r := 0; r < spec.patternReps; r++ {
		if r > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(period)
	}
	patternText := sb.String()
	srcs = append(srcs, reuse.Source{ID: 2, Text: patternText})

	// 其余文档：随机填充。
	per := spec.fillerWords / spec.fillerDocs
	for d := 0; d < spec.fillerDocs; d++ {
		n := per
		if d == spec.fillerDocs-1 {
			n += spec.fillerWords - per*spec.fillerDocs
		}
		var fb strings.Builder
		for w := 0; w < n; w++ {
			if w > 0 {
				fb.WriteByte(' ')
			}
			fb.WriteString(fillerWord(rng.Intn(spec.vocab)))
		}
		srcs = append(srcs, reuse.Source{ID: uint32(3 + d), Text: fb.String()})
	}
	return srcs, patternText
}

// memSampler 周期采样堆内存，记录峰值。
type memSampler struct {
	peak uint64
	stop chan struct{}
	done chan struct{}
}

func startMemSampler() *memSampler {
	m := &memSampler{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(m.done)
		t := time.NewTicker(2 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-t.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				if ms.HeapInuse+ms.StackInuse > m.peak {
					m.peak = ms.HeapInuse + ms.StackInuse
				}
			}
		}
	}()
	return m
}

func (m *memSampler) report() float64 {
	close(m.stop)
	<-m.done
	return float64(m.peak) / mib
}

func maxRSSMiB() float64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return float64(ru.Maxrss) / mib // darwin 上单位为字节
}

func heapInuseMiB() float64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapInuse) / mib
}

func main() {
	var (
		dir   = flag.String("dir", "", "语料目录（默认临时目录）")
		keep  = flag.Bool("keep", false, "保留语料目录")
		quick = flag.Bool("quick", false, "缩小 10 倍快速自检")
	)
	flag.Parse()

	spec := corpusSpec{
		sharedRun:   1_000_007,
		patternLen:  997,
		patternReps: 1500,
		fillerDocs:  19_998,
		fillerWords: 2_504_493,
		vocab:       5000,
	}
	q1, q2 := 10_000, 100_000
	if *quick {
		spec.sharedRun = 100_007
		spec.patternReps = 150
		spec.fillerDocs = 2_000
		spec.fillerWords = 250_000
		q1, q2 = 1_000, 10_000
	}

	if *dir == "" {
		tmp, err := os.MkdirTemp("", "reuse-scalecheck")
		if err != nil {
			panic(err)
		}
		*dir = tmp
	}
	if !*keep {
		defer os.RemoveAll(*dir)
	}

	sampler := startMemSampler()
	totalStart := time.Now()

	// ---- 构建语料 ----
	srcs, patternText := buildCorpus(spec, 42)
	totalWords := spec.sharedRun + spec.patternLen*spec.patternReps + spec.fillerWords
	totalBytes := 0
	for _, s := range srcs {
		totalBytes += len(s.Text)
	}
	fmt.Printf("corpus: %d sources, %d words, %.1f MiB text (limit 64 MiB)\n",
		len(srcs), totalWords, float64(totalBytes)/mib)

	t0 := time.Now()
	if err := reuse.Build(*dir, srcs); err != nil {
		panic(err)
	}
	buildDur := time.Since(t0)
	srcs = nil
	fmt.Printf("build: %v (live heap %.1f MiB)\n", buildDur.Round(time.Millisecond), heapInuseMiB())

	t0 = time.Now()
	c, err := reuse.Open(*dir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("open: %v (live heap %.1f MiB)\n", time.Since(t0).Round(time.Millisecond), heapInuseMiB())
	defer c.Close()

	// ---- 查询 ----
	sharedWordText := func(n int) string { return strings.Repeat("shared ", n-1) + "shared" }
	patternWords := strings.Split(patternText, " ")

	failures := 0
	check := func(name string, got, want []reuse.Evidence, dur time.Duration, budget time.Duration) {
		ok := len(got) == len(want)
		if ok {
			for i := range want {
				if got[i] != want[i] {
					ok = false
					break
				}
			}
		}
		status := "PASS"
		if !ok {
			status = "FAIL"
			failures++
		}
		if dur > budget {
			status = "SLOW"
			failures++
		}
		fmt.Printf("%s: %d evidence(s) in %v (budget %v) [%s]\n",
			name, len(got), dur.Round(time.Millisecond), budget, status)
		if !ok {
			fmt.Printf("  got:  %+v\n  want: %+v\n", got, want)
		}
	}

	// 查询 1：纯公共前缀，1 万词。
	{
		q := sharedWordText(q1)
		t0 = time.Now()
		got := c.Query(q)
		dur := time.Since(t0)
		want := []reuse.Evidence{{
			SourceID:    1,
			QueryStart:  0,
			QueryEnd:    utf8.RuneCountInString(q),
			SourceStart: 0,
			SourceEnd:   utf8.RuneCountInString(q),
		}}
		check(fmt.Sprintf("query shared %d words", q1), got, want, dur, 3*time.Second)
	}

	// 查询 2 与 3：公共前缀 + 周期片段混合，1 万词与 10 万词。
	for _, total := range []int{q1, q2} {
		half := total / 2
		part1 := strings.Repeat("shared ", half)
		part2 := strings.Join(patternWords[:total-half], " ")
		q := part1 + part2
		sharedRunes := half * 7 // "shared " 为 7 码点
		part2Runes := utf8.RuneCountInString(part2)
		t0 = time.Now()
		got := c.Query(q)
		dur := time.Since(t0)
		want := []reuse.Evidence{
			{SourceID: 1, QueryStart: 0, QueryEnd: sharedRunes - 1, SourceStart: 0, SourceEnd: sharedRunes - 1},
			{SourceID: 2, QueryStart: sharedRunes, QueryEnd: sharedRunes + part2Runes, SourceStart: 0, SourceEnd: part2Runes},
		}
		budget := 3 * time.Second
		if total > 10_000 {
			budget = 10 * time.Second
		}
		check(fmt.Sprintf("query mixed %d words", total), got, want, dur, budget)
	}

	// 查询 4：周期文档内部的跨周期长命中。
	{
		start := spec.patternLen / 2
		length := q1
		if length > spec.patternLen*spec.patternReps-start {
			length = spec.patternLen*spec.patternReps - start
		}
		q := strings.Join(patternWords[start:start+length], " ")
		t0 = time.Now()
		got := c.Query(q)
		dur := time.Since(t0)
		want := []reuse.Evidence{{
			SourceID:    2,
			QueryStart:  0,
			QueryEnd:    utf8.RuneCountInString(q),
			SourceStart: runeOffsetOfWord(patternText, start),
			SourceEnd:   runeOffsetOfWord(patternText, start) + utf8.RuneCountInString(q),
		}}
		check(fmt.Sprintf("query periodic %d words", length), got, want, dur, 3*time.Second)
	}

	peak := sampler.report()
	fmt.Printf("peak heap+stack: %.1f MiB (limit 512 MiB), max RSS: %.1f MiB\n", peak, maxRSSMiB())
	fmt.Printf("total: %v\n", time.Since(totalStart).Round(time.Millisecond))
	if failures > 0 {
		fmt.Printf("scalecheck: %d FAILURE(S)\n", failures)
		os.Exit(1)
	}
	fmt.Println("scalecheck: ALL PASS")
}

// runeOffsetOfWord 返回第 i 个词（空格分隔）在文本中的起始码点偏移。
func runeOffsetOfWord(text string, i int) int {
	off := 0
	for k := 0; k < i; k++ {
		idx := strings.IndexByte(text[byteOffsetOfRune(text, off):], ' ')
		if idx < 0 {
			return utf8.RuneCountInString(text)
		}
		off += utf8.RuneCountInString(text[byteOffsetOfRune(text, off):byteOffsetOfRune(text, off)+idx]) + 1
	}
	return off
}

func byteOffsetOfRune(s string, r int) int {
	off := 0
	for i := 0; i < r; i++ {
		_, size := utf8.DecodeRuneInString(s[off:])
		off += size
	}
	return off
}
