package reuse

// 双滚动哈希，模 2^64 自然溢出。两组独立基数与盐，
// 组合冲突概率约 2^-128，工程上可忽略。
const (
	hashBase1 uint64 = 0x9E3779B97F4A7C15
	hashBase2 uint64 = 0xC2B2AE3D27D4EB4F
	hashSalt1 uint64 = 0x2545F4914F6CDD1D
	hashSalt2 uint64 = 0x165667B19E3779F9
)

// hashPowers 返回 base 的 0..n 次幂。
func hashPowers(n int, base uint64) []uint64 {
	pow := make([]uint64, n+1)
	pow[0] = 1
	for i := 1; i <= n; i++ {
		pow[i] = pow[i-1] * base
	}
	return pow
}

// prefixHash 计算词 ID 序列的前缀哈希，h[k] 为前 k 个词的哈希，长度为 len(ids)+1。
func prefixHash(ids []uint32, base, salt uint64) []uint64 {
	h := make([]uint64, len(ids)+1)
	for i, id := range ids {
		h[i+1] = h[i]*base + uint64(id) + salt
	}
	return h
}

// subHash 返回序列区间 [l, r) 的哈希值。
func subHash(h []uint64, pow []uint64, l, r int) uint64 {
	return h[r] - h[l]*pow[r-l]
}
