package reuse

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
)

// MinMatchWords 是形成证据所需的最短连续复用词数。
const MinMatchWords = 8

// 滚动哈希参数（两组独立基），固定以保证可复现。
// 双 64 位哈希联合使用，碰撞概率可忽略。
const (
	hashBase1 = uint64(0x9e3779b97f4a7c15)
	hashBase2 = uint64(0xc2b2ae3d27d4eb4f)
	hashSalt  = uint64(0x517cc1b727220a95)
)

type sourceDoc struct {
	id    int
	words []Word   // 词在原文中的码点区间
	toks  []uint32 // 词的词表 ID
}

// Builder 用于整批构建只读语料索引。
type Builder struct {
	docs  []sourceDoc
	idSet map[int]bool
	vocab *vocab
}

// NewBuilder 创建语料构建器。
func NewBuilder() *Builder {
	return &Builder{idSet: make(map[int]bool), vocab: newVocab()}
}

// AddDocument 加入一份来源文档，id 为唯一正整数。
// 文本在加入时即完成分词与词表登记，之后不再保留原文。
func (b *Builder) AddDocument(id int, text string) error {
	if id <= 0 {
		return fmt.Errorf("reuse: 来源编号必须为正整数, got %d", id)
	}
	if b.idSet[id] {
		return fmt.Errorf("reuse: 来源编号重复: %d", id)
	}
	b.idSet[id] = true
	words, buf, offs := tokenizeLower(text)
	toks := make([]uint32, len(words))
	for i := range toks {
		toks[i] = b.vocab.intern(buf[offs[i]:offs[i+1]])
	}
	b.docs = append(b.docs, sourceDoc{id: id, words: words, toks: toks})
	return nil
}

// Index 是构建完成后的只读语料索引。
type Index struct {
	vocab *vocab

	tokens  []uint32 // 词 ID 流，文档间以 sepID 分隔
	sepID   uint32
	sa      []int32
	rmq     *rangeMin
	corpusH []uint64 // 前缀哈希（基 1），len = len(tokens)+1
	corpusG []uint64 // 前缀哈希（基 2）
	pow1    []uint64 // hashBase1 的幂，len = len(tokens)+1
	pow2    []uint64 // hashBase2 的幂

	docTokenStart []int32 // 内部文档序号 -> 首个 token 位置
	docWordStart  []int32 // 内部文档序号 -> 全局词数组中的起始下标
	docIDs        []int   // 内部文档序号 -> 外部编号（升序）

	wordRuneStart []int32 // 全局词下标 -> 所在文档原文码点区间
	wordRuneEnd   []int32
}

// Build 构建只读语料索引。构建后 Builder 不可再用。
func (b *Builder) Build() (*Index, error) {
	if len(b.docs) == 0 {
		return nil, errors.New("reuse: 语料为空")
	}
	docs := b.docs
	b.docs = nil
	sort.Slice(docs, func(i, j int) bool { return docs[i].id < docs[j].id })

	idx := &Index{vocab: b.vocab}
	totalWords := 0
	for i := range docs {
		totalWords += len(docs[i].toks)
	}
	idx.sepID = uint32(idx.vocab.size())

	nDocs := len(docs)
	idx.docTokenStart = make([]int32, nDocs)
	idx.docWordStart = make([]int32, nDocs)
	idx.docIDs = make([]int, nDocs)
	idx.tokens = make([]uint32, 0, totalWords+nDocs)
	idx.wordRuneStart = make([]int32, 0, totalWords)
	idx.wordRuneEnd = make([]int32, 0, totalWords)
	for i := range docs {
		idx.docIDs[i] = docs[i].id
		idx.docTokenStart[i] = int32(len(idx.tokens))
		idx.docWordStart[i] = int32(len(idx.wordRuneStart))
		for j, t := range docs[i].toks {
			idx.tokens = append(idx.tokens, t)
			idx.wordRuneStart = append(idx.wordRuneStart, int32(docs[i].words[j].Start))
			idx.wordRuneEnd = append(idx.wordRuneEnd, int32(docs[i].words[j].End))
		}
		// 文档末尾加分隔符，匹配不得跨文档边界。
		idx.tokens = append(idx.tokens, idx.sepID)
		docs[i].toks = nil
		docs[i].words = nil
	}

	n := len(idx.tokens)
	// 后缀数组构建的临时数组占用较大，先完成并释放再分配后续结构。
	idx.sa = buildSuffixArray(idx.tokens)
	runtime.GC()

	// RMQ 比较键：先内部文档序号（即编号升序），再 token 位置（即起始先后）。
	docOf := func(p int32) int {
		return sort.Search(nDocs, func(d int) bool {
			return d+1 >= nDocs || idx.docTokenStart[d+1] > p
		})
	}
	keys := make([]uint64, n)
	for j := 0; j < n; j++ {
		p := idx.sa[j]
		keys[j] = uint64(docOf(p))<<32 | uint64(uint32(p))
	}
	idx.rmq = newRangeMin(keys)
	runtime.GC()

	// 滚动哈希前缀（双基）。
	idx.corpusH = make([]uint64, n+1)
	idx.corpusG = make([]uint64, n+1)
	idx.pow1 = make([]uint64, n+1)
	idx.pow2 = make([]uint64, n+1)
	idx.pow1[0] = 1
	idx.pow2[0] = 1
	for i := 0; i < n; i++ {
		idx.pow1[i+1] = idx.pow1[i] * hashBase1
		idx.pow2[i+1] = idx.pow2[i] * hashBase2
		idx.corpusH[i+1] = idx.corpusH[i]*hashBase1 + uint64(idx.tokens[i]) + hashSalt
		idx.corpusG[i+1] = idx.corpusG[i]*hashBase2 + uint64(idx.tokens[i]) + hashSalt
	}
	return idx, nil
}

// docOfToken 返回 token 位置所属的内部文档序号。
func (idx *Index) docOfToken(p int32) int {
	return sort.Search(len(idx.docTokenStart), func(d int) bool {
		return d+1 >= len(idx.docTokenStart) || idx.docTokenStart[d+1] > p
	})
}
