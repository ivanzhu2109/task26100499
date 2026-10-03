package reuse

// MinEvidenceWords 是形成证据的最小连续复用长度（词数）。
const MinEvidenceWords = 8

// Evidence 是一段连续复用证据。
// 坐标均为 Unicode 码点偏移，从 0 开始、左闭右开；
// 区间覆盖首词首字符至末词末字符，包含中间分隔符。
type Evidence struct {
	SourceID    uint32 // 来源文档编号
	QueryStart  int    // 待查文本区间起点
	QueryEnd    int    // 待查文本区间终点
	SourceStart int    // 来源文本区间起点
	SourceEnd   int    // 来源文本区间终点
}

// queryCtx 保存单次查询的预计算数据。
type queryCtx struct {
	ids   []uint32
	h1    []uint64
	h2    []uint64
	pow1  []uint64
	pow2  []uint64
	known []bool
}

// Query 在待查文本中定位连续复用证据，按待查起点升序返回。
//
// 对每个词起点求能在单一来源中逐词连续出现的最长片段（不足 8 词忽略），
// 并列时取编号最小、再取起始位置最早的来源出现；
// 最后丢弃被其他候选区间严格包含的证据。
func (c *Corpus) Query(text string) []Evidence {
	spans, words := tokenizeLower(text)
	m := len(spans)
	if m < MinEvidenceWords || c.n == 0 {
		return nil
	}
	q := &queryCtx{ids: make([]uint32, m), known: make([]bool, m)}
	nextID := uint32(c.vocab.size() + 1)
	for i, w := range words {
		if id := c.vocab.id(w); id > 0 {
			q.ids[i] = id
			q.known[i] = true
		} else {
			q.ids[i] = nextID // 未登录词：唯一新 ID，永不命中
			nextID++
		}
	}
	q.h1 = prefixHash(q.ids, hashBase1, hashSalt1)
	q.h2 = prefixHash(q.ids, hashBase2, hashSalt2)
	// 哈希幂次只需到查询长度（LCE 上限不超过查询剩余词数）。
	q.pow1 = hashPowers(m, hashBase1)
	q.pow2 = hashPowers(m, hashBase2)

	var ev []Evidence
	maxEnd := 0 // 已有候选的最大结束词下标，用于严格包含过滤
	for i := 0; i+MinEvidenceWords <= m; i++ {
		if !q.known[i] {
			continue
		}
		length, srcID, srcPos := c.longestAt(q, i)
		if length < MinEvidenceWords {
			continue
		}
		if i+length <= maxEnd {
			continue // 被更早起点且终点不更小的候选严格包含
		}
		maxEnd = i + length
		ev = append(ev, Evidence{
			SourceID:    srcID,
			QueryStart:  spans[i].start,
			QueryEnd:    spans[i+length-1].end,
			SourceStart: int(c.runeStart[srcPos]),
			SourceEnd:   int(c.runeEnd[int(srcPos)+length-1]),
		})
	}
	return ev
}

// longestAt 求查询词起点 qi 的最长连续匹配：
// 返回匹配长度（词数）、并列时编号最小的来源及其最早起始位置。
func (c *Corpus) longestAt(q *queryCtx, qi int) (int, uint32, uint32) {
	r := c.lowerBound(q, qi)
	length := 0
	if r > 0 {
		length = c.lce(int(c.sa[r-1]), q, qi)
	}
	if r < c.n {
		if l := c.lce(int(c.sa[r]), q, qi); l > length {
			length = l
		}
	}
	if length < MinEvidenceWords {
		return length, 0, 0
	}
	// 所有 lce >= length 的语料后缀在 SA 序上构成连续区间 [lo, hi)。
	lo := 0
	{ // 左界：[0, r) 上 lce 向 r 单调不减，找首个 lce >= length
		a, b := 0, r
		for a < b {
			mid := (a + b) / 2
			if c.lce(int(c.sa[mid]), q, qi) >= length {
				b = mid
			} else {
				a = mid + 1
			}
		}
		lo = a
	}
	hi := c.n
	{ // 右界：[r, n) 上 lce 单调不增，找首个 lce < length
		a, b := r, c.n
		for a < b {
			mid := (a + b) / 2
			if c.lce(int(c.sa[mid]), q, qi) >= length {
				a = mid + 1
			} else {
				b = mid
			}
		}
		hi = a
	}
	best := c.occMin(lo, hi)
	return length, uint32(best >> 32), uint32(best)
}

// lowerBound 返回查询后缀 qi 在语料后缀数组中的插入位置。
func (c *Corpus) lowerBound(q *queryCtx, qi int) int {
	lo, hi := 0, c.n
	for lo < hi {
		mid := (lo + hi) / 2
		if c.cmpSuffix(int(c.sa[mid]), q, qi) >= 0 {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// cmpSuffix 比较语料后缀 cp 与查询后缀 qi：
// -1 语料后缀小，+1 查询后缀小，0 完全相等。
func (c *Corpus) cmpSuffix(cp int, q *queryCtx, qi int) int {
	l := c.lce(cp, q, qi)
	qRem := len(q.ids) - qi
	cRem := c.n - cp
	if l == qRem && l == cRem {
		return 0
	}
	if l == cRem {
		return -1 // 语料后缀是查询后缀的前缀
	}
	if l == qRem {
		return 1 // 查询后缀是语料后缀的前缀
	}
	if c.ids[cp+l] < q.ids[qi+l] {
		return -1
	}
	return 1
}

// lce 求语料位置 cp 与查询位置 qi 的最长公共前缀（词数），双哈希判定。
func (c *Corpus) lce(cp int, q *queryCtx, qi int) int {
	limit := c.n - cp
	if rem := len(q.ids) - qi; rem < limit {
		limit = rem
	}
	lo, hi := 0, limit
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if subHash(c.h1, q.pow1, cp, cp+mid) == subHash(q.h1, q.pow1, qi, qi+mid) &&
			subHash(c.h2, q.pow2, cp, cp+mid) == subHash(q.h2, q.pow2, qi, qi+mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}
