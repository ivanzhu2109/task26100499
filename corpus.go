package reuse

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Corpus 是构建完成后的只读语料，从本地文件加载，可安全并发查询。
type Corpus struct {
	n         int
	vocab     *vocab
	ids       []uint32 // 语料词 ID 序列（含分隔符 0）
	sa        []uint32
	h1, h2    []uint64 // 语料前缀哈希（双哈希）
	occTree   []uint64 // SA 序上 (srcID<<32|pos) 的区间最小线段树（叶层从下标 n 起）
	runeStart []uint32 // 每个语料位置的词在所属来源中的起始码点偏移
	runeEnd   []uint32 // 每个语料位置的词在所属来源中的结束码点偏移
	docs      []docMeta
}

// Open 从 dir 加载 Build 生成的只读语料。
func Open(dir string) (*Corpus, error) {
	c := &Corpus{}
	meta, err := os.Open(filepath.Join(dir, fileMeta))
	if err != nil {
		return nil, err
	}
	defer meta.Close()
	br := bufio.NewReader(meta)
	magic := make([]byte, len(metaMagic))
	if _, err := io.ReadFull(br, magic); err != nil || string(magic) != metaMagic {
		return nil, fmt.Errorf("reuse: %s is not a reuse corpus", dir)
	}
	var head [3]uint32
	if err := binary.Read(br, binary.LittleEndian, &head); err != nil {
		return nil, err
	}
	c.n = int(head[0])
	docCount := int(head[1])

	if c.vocab, err = readVocabFile(dir); err != nil {
		return nil, err
	}
	if c.ids, err = readUint32s(dir, fileWords, c.n); err != nil {
		return nil, err
	}
	if c.sa, err = readUint32s(dir, fileSA, c.n); err != nil {
		return nil, err
	}
	if c.h1, err = readUint64s(dir, fileHash1, c.n+1); err != nil {
		return nil, err
	}
	if c.h2, err = readUint64s(dir, fileHash2, c.n+1); err != nil {
		return nil, err
	}
	runes, err := readUint32s(dir, fileRunes, 2*c.n)
	if err != nil {
		return nil, err
	}
	c.runeStart = runes[:c.n]
	c.runeEnd = runes[c.n:]
	docFlat, err := readUint32s(dir, fileDocs, 3*docCount)
	if err != nil {
		return nil, err
	}
	c.docs = make([]docMeta, docCount)
	for i := range c.docs {
		c.docs[i] = docMeta{id: docFlat[3*i], wordStart: docFlat[3*i+1], wordCount: docFlat[3*i+2]}
	}

	if err := c.loadOccTree(dir); err != nil {
		return nil, err
	}
	return c, nil
}

// loadOccTree 读取 occ 文件并就地构建 (srcID<<32|pos) 的区间最小线段树，
// 采用任意 n 的迭代布局，仅占 2n 空间。
func (c *Corpus) loadOccTree(dir string) error {
	n := c.n
	c.occTree = make([]uint64, 2*n)
	f, err := os.Open(filepath.Join(dir, fileOcc))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := binary.Read(bufio.NewReader(f), binary.LittleEndian, c.occTree[n:]); err != nil {
		return err
	}
	for i := n - 1; i >= 1; i-- {
		if c.occTree[2*i] < c.occTree[2*i+1] {
			c.occTree[i] = c.occTree[2*i]
		} else {
			c.occTree[i] = c.occTree[2*i+1]
		}
	}
	return nil
}

// occMin 返回 SA 序区间 [lo, hi) 内的最小 (srcID<<32|pos)。
func (c *Corpus) occMin(lo, hi int) uint64 {
	res := ^uint64(0)
	l, r := lo+c.n, hi+c.n
	for l < r {
		if l&1 == 1 {
			if c.occTree[l] < res {
				res = c.occTree[l]
			}
			l++
		}
		if r&1 == 1 {
			r--
			if c.occTree[r] < res {
				res = c.occTree[r]
			}
		}
		l >>= 1
		r >>= 1
	}
	return res
}

// Close 释放语料占用的内存（切片交由 GC 回收）。
func (c *Corpus) Close() error {
	*c = Corpus{}
	return nil
}

func readVocabFile(dir string) (*vocab, error) {
	f, err := os.Open(filepath.Join(dir, fileVocab))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readVocab(bufio.NewReader(f))
}

func readUint32s(dir, name string, want int) ([]uint32, error) {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make([]uint32, want)
	if err := binary.Read(bufio.NewReader(f), binary.LittleEndian, data); err != nil {
		return nil, err
	}
	return data, nil
}

func readUint64s(dir, name string, want int) ([]uint64, error) {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make([]uint64, want)
	if err := binary.Read(bufio.NewReader(f), binary.LittleEndian, data); err != nil {
		return nil, err
	}
	return data, nil
}
