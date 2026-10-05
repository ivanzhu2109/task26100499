// scaletest 是连续复用定位库的规模验证入口。
// 覆盖两类对抗场景：同一 8 词前缀在语料中出现约百万次、长段周期性重复。
// 报告证据数量、耗时与峰值内存，并对照指标给出 PASS/FAIL。
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"reuse"
)

const memLimit = 512 << 20 // 512 MiB

var peak int64

func sampleMem(stop <-chan struct{}) {
	var ms runtime.MemStats
	for {
		select {
		case <-stop:
			return
		default:
		}
		runtime.ReadMemStats(&ms)
		if v := int64(ms.HeapAlloc); v > atomic.LoadInt64(&peak) {
			atomic.StoreInt64(&peak, v)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func mib(v int64) string { return fmt.Sprintf("%.1f MiB", float64(v)/(1<<20)) }

// prefixCorpus 生成 docs 份文档，每份为 reps 次同一 8 词前缀。
func prefixCorpus(docs, reps int) []string {
	phrase := strings.Repeat("common ", 7) + "common"
	doc := strings.Repeat(phrase+" ", reps-1) + phrase
	out := make([]string, docs)
	for i := range out {
		out[i] = doc
	}
	return out
}

// periodicCorpus 生成 1000 词模式的长周期重复语料，切分为 docs 份。
func periodicCorpus(totalWords, docs int) []string {
	const period = 1000
	pattern := make([]string, period)
	for i := range pattern {
		pattern[i] = fmt.Sprintf("w%d", i)
	}
	perDoc := totalWords / docs
	out := make([]string, 0, docs)
	for start := 0; start+perDoc <= totalWords; start += perDoc {
		var sb strings.Builder
		for w := 0; w < perDoc; w++ {
			if w > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(pattern[(start+w)%period])
		}
		out = append(out, sb.String())
	}
	return out
}

// uniqueCorpus 生成全部不同的词（确定性乱序），切分为 docs 份。
func uniqueCorpus(totalWords, docs int) ([]string, []string) {
	words := make([]string, totalWords)
	for i := range words {
		words[i] = fmt.Sprintf("u%d", i)
	}
	// 确定性 Fisher-Yates 乱序。
	seed := uint64(12345)
	rnd := func() uint64 { seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17; return seed }
	for i := len(words) - 1; i > 0; i-- {
		j := int(rnd() % uint64(i+1))
		words[i], words[j] = words[j], words[i]
	}
	perDoc := totalWords / docs
	out := make([]string, 0, docs)
	for start := 0; start+perDoc <= len(words); start += perDoc {
		out = append(out, strings.Join(words[start:start+perDoc], " "))
	}
	return out, words
}

// periodicQuery 生成同一模式的查询文本。
func periodicQuery(words int) string {
	const period = 1000
	var sb strings.Builder
	for w := 0; w < words; w++ {
		if w > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "w%d", w%period)
	}
	return sb.String()
}

func prefixQuery(words int) string {
	reps := words / 8
	phrase := strings.Repeat("common ", 7) + "common"
	return strings.Repeat(phrase+" ", reps-1) + phrase
}

func main() {
	scenario := flag.String("scenario", "all", "all | prefix | periodic")
	corpusWords := flag.Int("corpus-words", 4800000, "语料总词数")
	maxDocs := flag.Int("docs", 20000, "来源文档数上限")
	flag.Parse()

	stop := make(chan struct{})
	defer close(stop)
	go sampleMem(stop)

	type queryCase struct {
		words int
		limit time.Duration
	}
	queries := []queryCase{{10000, 3 * time.Second}, {100000, 10 * time.Second}}

	scenarios := []string{"prefix", "periodic", "unique"}
	if *scenario != "all" {
		scenarios = []string{*scenario}
	}
	failed := false
	for _, sc := range scenarios {
		fmt.Printf("== 场景 %s ==\n", sc)
		var docs []string
		var uniqueWords []string
		var makeQuery func(int) string
		switch sc {
		case "prefix":
			reps := *corpusWords / 8 / *maxDocs
			if reps < 1 {
				reps = 1
			}
			docs = prefixCorpus(*maxDocs, reps)
			fmt.Printf("语料: %d 份文档, 每份 %d 词, 8 词公共前缀出现 %d 次\n",
				len(docs), reps*8, len(docs)*reps)
			makeQuery = prefixQuery
		case "periodic":
			docs = periodicCorpus(*corpusWords, *maxDocs)
			fmt.Printf("语料: %d 份文档, 1000 词模式周期重复\n", len(docs))
			makeQuery = periodicQuery
		case "unique":
			docs, uniqueWords = uniqueCorpus(*corpusWords, *maxDocs)
			fmt.Printf("语料: %d 份文档, %d 个词全不相同\n", len(docs), len(uniqueWords))
			makeQuery = func(n int) string {
				return strings.Join(uniqueWords[:n], " ")
			}
		}

		runtime.GC()
		atomic.StoreInt64(&peak, 0)
		t0 := time.Now()
		b := reuse.NewBuilder()
		for i, d := range docs {
			if err := b.AddDocument(i+1, d); err != nil {
				fmt.Println("AddDocument:", err)
				os.Exit(1)
			}
		}
		fmt.Printf("加入文档: %v, 峰值内存 %s\n", time.Since(t0).Round(time.Millisecond), mib(atomic.LoadInt64(&peak)))
		runtime.GC()
		atomic.StoreInt64(&peak, 0)
		t0 = time.Now()
		idx, err := b.Build()
		if err != nil {
			fmt.Println("Build:", err)
			os.Exit(1)
		}
		docs = nil
		fmt.Printf("构建: %v, 峰值内存 %s\n", time.Since(t0).Round(time.Millisecond), mib(atomic.LoadInt64(&peak)))

		for _, q := range queries {
			text := makeQuery(q.words)
			runtime.GC()
			atomic.StoreInt64(&peak, 0)
			t1 := time.Now()
			ev := idx.Query(text)
			el := time.Since(t1)
			mem := atomic.LoadInt64(&peak)
			ok := el <= q.limit && mem <= memLimit
			status := "PASS"
			if !ok {
				status = "FAIL"
				failed = true
			}
			fmt.Printf("查询 %d 词: 证据 %d 段, 耗时 %v (限 %v), 峰值内存 %s (限 512 MiB) [%s]\n",
				q.words, len(ev), el.Round(time.Millisecond), q.limit, mib(mem), status)
		}
	}
	if failed {
		os.Exit(1)
	}
}
