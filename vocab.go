package reuse

// vocab 是词 -> 稠密 ID 的开放寻址哈希表。
// 相比 map[string]uint32 显著降低百万级不同词时的内存：
// 词字节集中存放在 arena，表内只存 ID，哈希只存 32 位指纹。
type vocab struct {
	arena   []byte   // 所有词的字节依次拼接
	offsets []uint32 // id -> 词在 arena 中的起点，len = count+1
	hashes  []uint32 // id -> 词的哈希指纹（冲突时回退字节比较）
	table   []uint32 // 开放寻址，存 id+1，0 表示空
	mask    uint32
	count   int
}

func newVocab() *vocab {
	return &vocab{
		offsets: []uint32{0},
		table:   make([]uint32, 1<<10),
		mask:    1<<10 - 1,
	}
}

func hashWord(b []byte) uint64 {
	h := uint64(0xcbf29ce484222325)
	for _, c := range b {
		h ^= uint64(c)
		h *= 0x100000001b3
	}
	return h
}

func fingerprint(h uint64) uint32 { return uint32(h>>32) | 1 }

func (v *vocab) wordAt(id uint32) []byte {
	return v.arena[v.offsets[id]:v.offsets[id+1]]
}

// find 返回词对应的槽位与是否已存在。
func (v *vocab) find(b []byte, h uint64) (uint32, bool) {
	fp := fingerprint(h)
	slot := uint32(h) & v.mask
	for {
		e := v.table[slot]
		if e == 0 {
			return slot, false
		}
		id := e - 1
		if v.hashes[id] == fp && string(v.wordAt(id)) == string(b) {
			return slot, true
		}
		slot = (slot + 1) & v.mask
	}
}

// lookup 查询词的 ID，不存在时 ok 为 false。
func (v *vocab) lookup(b []byte) (id uint32, ok bool) {
	slot, found := v.find(b, hashWord(b))
	if !found {
		return 0, false
	}
	return v.table[slot] - 1, true
}

// intern 返回词的 ID，不存在时分配新 ID。
func (v *vocab) intern(b []byte) uint32 {
	h := hashWord(b)
	slot, found := v.find(b, h)
	if found {
		return v.table[slot] - 1
	}
	if (v.count+1)*10 >= len(v.table)*7 { // 负载因子 0.7
		v.grow()
		slot, _ = v.find(b, h)
	}
	id := uint32(v.count)
	v.count++
	v.arena = append(v.arena, b...)
	v.offsets = append(v.offsets, uint32(len(v.arena)))
	v.hashes = append(v.hashes, fingerprint(h))
	v.table[slot] = id + 1
	return id
}

func (v *vocab) grow() {
	old := v.table
	v.table = make([]uint32, 2*len(old))
	v.mask = uint32(len(v.table)) - 1
	for _, e := range old {
		if e == 0 {
			continue
		}
		id := e - 1
		// 只存了指纹，完整哈希从词字节重新计算。
		slot := uint32(hashWord(v.wordAt(id))) & v.mask
		for v.table[slot] != 0 {
			slot = (slot + 1) & v.mask
		}
		v.table[slot] = e
	}
}

// size 返回不同词数量。
func (v *vocab) size() int { return v.count }
