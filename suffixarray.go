package reuse

// buildSuffixArray 对词 ID 序列构造后缀数组（倍增法 + 计数排序，O(n log n)）。
// ids 取值必须稠密（0..V），返回的 sa 满足 ids[sa[i]:] 按字典序升序。
func buildSuffixArray(ids []uint32) []uint32 {
	n := len(ids)
	if n == 0 {
		return nil
	}
	sa := make([]uint32, n)
	rank := make([]uint32, n)
	counts := make([]uint32, n+2)

	// 初始排名：rank = id+1（0 保留给越界位置）。
	maxRank := uint32(0)
	for i, id := range ids {
		r := id + 1
		rank[i] = r
		counts[r]++
		if r > maxRank {
			maxRank = r
		}
	}
	sum := uint32(0)
	for i := uint32(0); i <= maxRank; i++ {
		c := counts[i]
		counts[i] = sum
		sum += c
	}
	for i := 0; i < n; i++ {
		r := rank[i]
		sa[counts[r]] = uint32(i)
		counts[r]++
	}
	numRanks := int(maxRank)

	tmp := make([]uint32, n)
	newRank := make([]uint32, n)
	for k := 1; numRanks < n; k *= 2 {
		// 计数排序：第二关键字 rank[i+k]（越界为 0）。
		for i := 0; i <= numRanks; i++ {
			counts[i] = 0
		}
		for i := 0; i < n; i++ {
			var key uint32
			if i+k < n {
				key = rank[i+k]
			}
			counts[key]++
		}
		sum = 0
		for i := 0; i <= numRanks; i++ {
			c := counts[i]
			counts[i] = sum
			sum += c
		}
		for i := 0; i < n; i++ {
			var key uint32
			if i+k < n {
				key = rank[i+k]
			}
			tmp[counts[key]] = uint32(i)
			counts[key]++
		}
		// 计数排序：第一关键字 rank[i]（稳定）。
		for i := 0; i <= numRanks; i++ {
			counts[i] = 0
		}
		for i := 0; i < n; i++ {
			counts[rank[tmp[i]]]++
		}
		sum = 0
		for i := 0; i <= numRanks; i++ {
			c := counts[i]
			counts[i] = sum
			sum += c
		}
		for i := 0; i < n; i++ {
			s := tmp[i]
			key := rank[s]
			sa[counts[key]] = s
			counts[key]++
		}
		// 重新排名。
		newRank[sa[0]] = 1
		r := uint32(1)
		for i := 1; i < n; i++ {
			a, b := sa[i-1], sa[i]
			ka1, kb1 := rank[a], rank[b]
			var ka2, kb2 uint32
			if int(a)+k < n {
				ka2 = rank[int(a)+k]
			}
			if int(b)+k < n {
				kb2 = rank[int(b)+k]
			}
			if ka1 != kb1 || ka2 != kb2 {
				r++
			}
			newRank[b] = r
		}
		rank, newRank = newRank, rank
		numRanks = int(r)
	}
	return sa
}
