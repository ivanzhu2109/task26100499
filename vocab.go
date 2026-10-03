package reuse

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"
)

// vocab 保存去重排序后的词表。词 ID = 表内序号 + 1（0 保留为文档分隔符）。
type vocab struct {
	blob []byte   // 所有词连接后的字节
	off  []uint32 // 每个词在 blob 中的起始偏移，长度为词数+1
}

// buildVocab 从全部词（可重复，存于 blob，off 为各词起止偏移）构建排序去重词表，
// 并返回每个输入词对应的 ID（从 1 开始）。
func buildVocab(blob []byte, off []uint32) (*vocab, []uint32) {
	n := len(off) - 1
	wordAt := func(i int) []byte { return blob[off[i]:off[i+1]] }
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return bytes.Compare(wordAt(order[a]), wordAt(order[b])) < 0
	})
	v := &vocab{off: []uint32{0}}
	ids := make([]uint32, n)
	var prev []byte
	var curID uint32
	for _, idx := range order {
		w := wordAt(idx)
		if curID == 0 || !bytes.Equal(w, prev) {
			curID = uint32(len(v.off)) // 序号从 1 开始
			v.blob = append(v.blob, w...)
			v.off = append(v.off, uint32(len(v.blob)))
			prev = v.blob[v.off[curID-1]:v.off[curID]]
		}
		ids[idx] = curID
	}
	return v, ids
}

// id 返回词对应的 ID，未收录返回 0。
func (v *vocab) id(word []byte) uint32 {
	lo, hi := 0, v.size()
	for lo < hi {
		mid := (lo + hi) / 2
		c := bytes.Compare(v.word(uint32(mid+1)), word)
		if c == 0 {
			return uint32(mid + 1)
		} else if c < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return 0
}

// word 返回 ID 对应的词（id 从 1 开始）。
func (v *vocab) word(id uint32) []byte {
	return v.blob[v.off[id-1]:v.off[id]]
}

func (v *vocab) size() int { return len(v.off) - 1 }

// writeTo 序列化词表：词数、各词偏移、字节块。
func (v *vocab) writeTo(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(v.size())); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, v.off); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, v.blob)
}

func readVocab(r io.Reader) (*vocab, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	v := &vocab{off: make([]uint32, n+1)}
	if err := binary.Read(r, binary.LittleEndian, &v.off); err != nil {
		return nil, err
	}
	v.blob = make([]byte, v.off[n])
	return v, binary.Read(r, binary.LittleEndian, &v.blob)
}
