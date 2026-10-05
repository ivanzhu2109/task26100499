package reuse

// rmqBlockSize 是分块大小；块内直接扫描，块间用稀疏表。
const rmqBlockSize = 64

// rangeMin 支持对后缀数组区间做比较键最小值查询，返回下标。
// 内存约为满线段树的 1/13：块最小值的稀疏表 + 键数组。
type rangeMin struct {
	keys     []uint64
	blockMin []uint32   // 每块内最小键的下标
	sparse   [][]uint32 // blockMin 的稀疏表，sparse[k][i] 覆盖 2^k 个块
	log2     []int
}

// newRangeMin 以 keys[i] 作为后缀数组第 i 项的比较键。
func newRangeMin(keys []uint64) *rangeMin {
	n := len(keys)
	nBlocks := (n + rmqBlockSize - 1) / rmqBlockSize
	blockMin := make([]uint32, nBlocks)
	for b := 0; b < nBlocks; b++ {
		lo := b * rmqBlockSize
		hi := min(lo+rmqBlockSize, n)
		best := lo
		for i := lo + 1; i < hi; i++ {
			if keys[i] < keys[best] {
				best = i
			}
		}
		blockMin[b] = uint32(best)
	}
	log2 := make([]int, nBlocks+1)
	for i := 2; i <= nBlocks; i++ {
		log2[i] = log2[i/2] + 1
	}
	sparse := [][]uint32{blockMin}
	for k := 1; 1<<k <= nBlocks; k++ {
		prev := sparse[k-1]
		size := nBlocks - (1 << k) + 1
		cur := make([]uint32, size)
		half := 1 << (k - 1)
		for i := 0; i < size; i++ {
			cur[i] = minKeyIdx(keys, prev[i], prev[i+half])
		}
		sparse = append(sparse, cur)
	}
	return &rangeMin{keys: keys, blockMin: blockMin, sparse: sparse, log2: log2}
}

func minKeyIdx(keys []uint64, a, b uint32) uint32 {
	if keys[a] <= keys[b] {
		return a
	}
	return b
}

// argMin 返回 [lo, hi) 中键最小的下标；空区间返回 -1。
// 键已编码全部并列维度，不存在并列。
func (r *rangeMin) argMin(lo, hi int) int {
	if lo >= hi {
		return -1
	}
	best := -1
	scan := func(a, b int) {
		for i := a; i < b; i++ {
			if best < 0 || r.keys[i] < r.keys[best] {
				best = i
			}
		}
	}
	// 两端不足一块的部分直接扫描。
	headEnd := min(hi, (lo+rmqBlockSize-1)/rmqBlockSize*rmqBlockSize)
	scan(lo, headEnd)
	if headEnd < hi {
		tailStart := hi / rmqBlockSize * rmqBlockSize
		if tailStart > headEnd {
			// 中间完整块用稀疏表。
			bLo := headEnd / rmqBlockSize
			bHi := hi / rmqBlockSize
			k := r.log2[bHi-bLo]
			cand := minKeyIdx(r.keys, r.sparse[k][bLo], r.sparse[k][bHi-(1<<k)])
			if best < 0 || r.keys[cand] < r.keys[best] {
				best = int(cand)
			}
		}
		scan(max(tailStart, headEnd), hi)
	}
	return best
}
