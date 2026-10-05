package reuse

import "sort"

// Evidence 是一段连续复用证据，坐标为码点偏移，左闭右开。
type Evidence struct {
	DocID       int // 来源文档编号
	QueryStart  int // 待查文本区间起点（码点）
	QueryEnd    int // 待查文本区间终点（码点）
	SourceStart int // 来源文本区间起点（码点）
	SourceEnd   int // 来源文本区间终点（码点）
}

// candidate 是某待查词起点的最长匹配。
type candidate struct {
	qWord  int // 待查词下标
	length int // 匹配词数
	doc    int // 内部文档序号
	tokPos int // 语料 token 位置
}

// Query 在语料中定位待查文本的连续复用证据，按待查起点升序返回。
func (idx *Index) Query(text string) []Evidence {
	qWords, qBuf, qOffs := tokenizeLower(text)
	qn := len(qWords)
	if qn == 0 {
		return nil
	}
	// 待查词 -> token ID；语料没有的词分配互不相同的伪 ID（不等于任何语料 token）。
	qTokens := make([]uint32, qn)
	fakeID := idx.sepID
	unknown := make(map[string]uint32)
	for i := 0; i < qn; i++ {
		w := qBuf[qOffs[i]:qOffs[i+1]]
		if id, ok := idx.vocab.lookup(w); ok {
			qTokens[i] = id
		} else {
			id, ok := unknown[string(w)]
			if !ok {
				fakeID++
				id = fakeID
				unknown[string(w)] = id
			}
			qTokens[i] = id
		}
	}
	pow1, pow2 := idx.pow1, idx.pow2
	if qn+1 > len(pow1) {
		pow1 = make([]uint64, qn+1)
		pow2 = make([]uint64, qn+1)
		pow1[0] = 1
		pow2[0] = 1
		for i := 0; i < qn; i++ {
			pow1[i+1] = pow1[i] * hashBase1
			pow2[i+1] = pow2[i] * hashBase2
		}
	}
	qH := make([]uint64, qn+1)
	qG := make([]uint64, qn+1)
	for i := 0; i < qn; i++ {
		qH[i+1] = qH[i]*hashBase1 + uint64(qTokens[i]) + hashSalt
		qG[i+1] = qG[i]*hashBase2 + uint64(qTokens[i]) + hashSalt
	}

	n := len(idx.tokens)
	corpusHash := func(a, b int) uint64 {
		return idx.corpusH[b] - idx.corpusH[a]*pow1[b-a]
	}
	corpusHash2 := func(a, b int) uint64 {
		return idx.corpusG[b] - idx.corpusG[a]*pow2[b-a]
	}
	queryHash := func(a, b int) uint64 {
		return qH[b] - qH[a]*pow1[b-a]
	}
	queryHash2 := func(a, b int) uint64 {
		return qG[b] - qG[a]*pow2[b-a]
	}
	// lcp 返回待查后缀 qi 与语料后缀 cp 的最长公共前缀词数，上限 limit。
	lcp := func(qi, cp, limit int) int {
		lo, hi := 0, limit
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if queryHash(qi, qi+mid) == corpusHash(cp, cp+mid) &&
				queryHash2(qi, qi+mid) == corpusHash2(cp, cp+mid) {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return lo
	}
	// cmpSuffix 比较待查后缀与语料后缀，返回 -1/0/1。
	cmpSuffix := func(qi, cp int) int {
		qRem, cRem := qn-qi, n-cp
		limit := qRem
		if cRem < limit {
			limit = cRem
		}
		l := lcp(qi, cp, limit)
		if l < limit {
			if qTokens[qi+l] < idx.tokens[cp+l] {
				return -1
			}
			return 1
		}
		switch {
		case qRem < cRem:
			return -1
		case qRem > cRem:
			return 1
		}
		return 0
	}
	// cmpPrefix 比较语料后缀与待查的 length 词前缀：
	// 语料后缀以该前缀开头记 0；语料后缀较短记 -1；否则按首个不同词比较。
	cmpPrefix := func(qi, length, cp int) int {
		cRem := n - cp
		limit := length
		if cRem < limit {
			limit = cRem
		}
		l := lcp(qi, cp, limit)
		if l < limit {
			if idx.tokens[cp+l] < qTokens[qi+l] {
				return -1
			}
			return 1
		}
		if cRem < length {
			return -1
		}
		return 0
	}

	var cands []candidate
	for qi := 0; qi+MinMatchWords <= qn; qi++ {
		if qTokens[qi] > idx.sepID {
			continue // 语料中不存在的词
		}
		// 待查后缀在后缀数组中的插入位置（首个不小于待查后缀的语料后缀）。
		k := sort.Search(n, func(j int) bool {
			return cmpSuffix(qi, int(idx.sa[j])) <= 0
		})
		best := 0
		if k < n {
			if l := lcp(qi, int(idx.sa[k]), min(qn-qi, n-int(idx.sa[k]))); l > best {
				best = l
			}
		}
		if k > 0 {
			if l := lcp(qi, int(idx.sa[k-1]), min(qn-qi, n-int(idx.sa[k-1]))); l > best {
				best = l
			}
		}
		if best < MinMatchWords {
			continue
		}
		// 所有以该最长前缀开头的语料后缀构成连续区间。
		lo := sort.Search(n, func(j int) bool {
			return cmpPrefix(qi, best, int(idx.sa[j])) >= 0
		})
		hi := sort.Search(n, func(j int) bool {
			return cmpPrefix(qi, best, int(idx.sa[j])) > 0
		})
		j := idx.rmq.argMin(lo, hi)
		if j < 0 {
			continue
		}
		p := int(idx.sa[j])
		cands = append(cands, candidate{
			qWord:  qi,
			length: best,
			doc:    idx.docOfToken(int32(p)),
			tokPos: p,
		})
	}

	// 过滤：仅保留未被其他候选区间严格包含的候选。
	// 起点唯一且递增，[a,b) 被严格包含当且仅当存在更早起点 c<a 且 end(c)>=b。
	var ev []Evidence
	maxEnd := 0
	for _, c := range cands {
		end := c.qWord + c.length
		if end <= maxEnd {
			continue
		}
		maxEnd = end
		w := idx.docWordStart[c.doc] + int32(c.tokPos) - idx.docTokenStart[c.doc]
		last := w + int32(c.length) - 1
		ev = append(ev, Evidence{
			DocID:       idx.docIDs[c.doc],
			QueryStart:  qWords[c.qWord].Start,
			QueryEnd:    qWords[end-1].End,
			SourceStart: int(idx.wordRuneStart[w]),
			SourceEnd:   int(idx.wordRuneEnd[last]),
		})
	}
	return ev
}
