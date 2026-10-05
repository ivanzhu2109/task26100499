package reuse

// buildSuffixArray 对词 ID 序列构建后缀数组。
// 词 ID 要求为稠密非负整数（语料词表 ID 连续，文档分隔符 ID 紧随其后）。
// 采用前缀倍增 + 基数排序，时间 O(n log n)，额外数组 O(n)。
func buildSuffixArray(tokens []uint32) []int32 {
	n := len(tokens)
	if n == 0 {
		return nil
	}
	sa := make([]int32, n)
	rank := make([]int32, n)
	m := 0
	for i, t := range tokens {
		sa[i] = int32(i)
		rank[i] = int32(t)
		if int(t) >= m {
			m = int(t) + 1
		}
	}
	tmp := make([]int32, n)
	cnt := make([]int32, n+1)
	// 按 key 对 sa 做稳定计数排序，键值须在 [0, buckets) 内。
	sortBy := func(key func(i int32) int32, buckets int) {
		for i := 0; i < buckets; i++ {
			cnt[i] = 0
		}
		for _, v := range sa {
			cnt[key(v)]++
		}
		sum := 0
		for i := 0; i < buckets; i++ {
			c := cnt[i]
			cnt[i] = int32(sum)
			sum += int(c)
		}
		for _, v := range sa {
			k := key(v)
			tmp[cnt[k]] = v
			cnt[k]++
		}
		copy(sa, tmp)
	}
	rankAt := func(i int32, k int) int32 {
		if j := int(i) + k; j < n {
			return rank[j]
		}
		return -1
	}
	sortBy(func(i int32) int32 { return rank[i] }, m)
	classes := m
	newRank := make([]int32, n)
	for k := 1; classes < n; k <<= 1 {
		kk := k
		// 第二键越界记 0，其余为 rank+1，保证越界最小。
		sortBy(func(i int32) int32 { return rankAt(i, kk) + 1 }, classes+1)
		sortBy(func(i int32) int32 { return rank[i] }, classes)
		newRank[sa[0]] = 0
		c := 1
		for idx := 1; idx < n; idx++ {
			a, b := sa[idx-1], sa[idx]
			if rank[a] != rank[b] || rankAt(a, kk) != rankAt(b, kk) {
				c++
			}
			newRank[b] = int32(c - 1)
		}
		rank, newRank = newRank, rank
		classes = c
	}
	return sa
}
