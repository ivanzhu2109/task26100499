package reuse

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// Source 是一份来源文档：唯一正整数编号 + UTF-8 纯文本。
type Source struct {
	ID   uint32
	Text string
}

const metaMagic = "reuse001"

// 语料文件布局：dir 下的下列文件。
const (
	fileMeta  = "meta"
	fileVocab = "vocab"
	fileWords = "words"
	fileSA    = "sa"
	fileHash1 = "hash1"
	fileHash2 = "hash2"
	fileOcc   = "occ"
	fileRunes = "runes"
	fileDocs  = "docs"
)

type docMeta struct {
	id        uint32
	wordStart uint32 // 在语料词序列中的起始位置
	wordCount uint32
}

// Build 将整批来源文档构建为只读语料，写入 dir 下的本地文件。
// 构建完成后语料只读，可用 Open 加载查询。
func Build(dir string, srcs []Source) error {
	if err := validateSources(srcs); err != nil {
		return err
	}

	// 第一遍：分词。词字节汇入共享 blob，码点区间按语料位置记录（分隔符占位 0）。
	var wordBlob []byte
	wordOff := []uint32{0}
	var runeStart, runeEnd []uint32
	docs := make([]docMeta, 0, len(srcs))
	for _, s := range srcs {
		spans, words := tokenizeLower(s.Text)
		docs = append(docs, docMeta{
			id:        s.ID,
			wordStart: uint32(len(runeStart)),
			wordCount: uint32(len(spans)),
		})
		for k, w := range words {
			wordBlob = append(wordBlob, w...)
			wordOff = append(wordOff, uint32(len(wordBlob)))
			runeStart = append(runeStart, uint32(spans[k].start))
			runeEnd = append(runeEnd, uint32(spans[k].end))
		}
		runeStart = append(runeStart, 0) // 分隔符占位
		runeEnd = append(runeEnd, 0)
	}

	// 词表：排序去重，词 ID 从 1 开始（0 保留为文档分隔符）。
	v, wordIDs := buildVocab(wordBlob, wordOff)
	wordBlob = nil

	// 拼接语料词序列：每份来源的词后跟一个分隔符，匹配不会跨文档边界。
	n := len(runeStart)
	ids := make([]uint32, 0, n)
	w := 0
	for i := range docs {
		for k := uint32(0); k < docs[i].wordCount; k++ {
			ids = append(ids, wordIDs[w])
			w++
		}
		ids = append(ids, 0) // 分隔符
	}
	wordIDs = nil

	sa := buildSuffixArray(ids)
	h1 := prefixHash(ids, hashBase1, hashSalt1)
	h2 := prefixHash(ids, hashBase2, hashSalt2)

	// 每个后缀数组槽位的最优来源 (srcID, pos)：分隔符位置记为最大值，永不命中。
	occ := make([]uint64, n)
	posDocID := make([]uint32, n)
	for _, d := range docs {
		for p := d.wordStart; p < d.wordStart+d.wordCount; p++ {
			posDocID[p] = d.id
		}
	}
	for j, p := range sa {
		pos := int(p)
		if ids[pos] == 0 {
			occ[j] = ^uint64(0)
			continue
		}
		occ[j] = uint64(posDocID[pos])<<32 | uint64(pos)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeMeta(dir, n, len(docs), v.size()); err != nil {
		return err
	}
	if err := writeVocabFile(dir, v); err != nil {
		return err
	}
	if err := writeUint32s(dir, fileWords, ids); err != nil {
		return err
	}
	ids = nil
	if err := writeUint32s(dir, fileSA, sa); err != nil {
		return err
	}
	if err := writeUint64s(dir, fileHash1, h1); err != nil {
		return err
	}
	if err := writeUint64s(dir, fileHash2, h2); err != nil {
		return err
	}
	if err := writeUint64s(dir, fileOcc, occ); err != nil {
		return err
	}
	if err := writeUint32Pairs(dir, fileRunes, runeStart, runeEnd); err != nil {
		return err
	}
	docFlat := make([]uint32, 0, 3*len(docs))
	for _, d := range docs {
		docFlat = append(docFlat, d.id, d.wordStart, d.wordCount)
	}
	return writeUint32s(dir, fileDocs, docFlat)
}

func validateSources(srcs []Source) error {
	seen := make(map[uint32]bool, len(srcs))
	for _, s := range srcs {
		if s.ID == 0 {
			return fmt.Errorf("reuse: source id must be a positive integer")
		}
		if seen[s.ID] {
			return fmt.Errorf("reuse: duplicate source id %d", s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}

func writeMeta(dir string, n, docCount, vocabSize int) error {
	f, err := os.Create(filepath.Join(dir, fileMeta))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := w.WriteString(metaMagic); err != nil {
		return err
	}
	for _, v := range []uint32{uint32(n), uint32(docCount), uint32(vocabSize)} {
		if err := binary.Write(w, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	return w.Flush()
}

func writeVocabFile(dir string, v *vocab) error {
	f, err := os.Create(filepath.Join(dir, fileVocab))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := v.writeTo(w); err != nil {
		return err
	}
	return w.Flush()
}

func writeUint32s(dir, name string, data []uint32) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := binary.Write(w, binary.LittleEndian, data); err != nil {
		return err
	}
	return w.Flush()
}

func writeUint64s(dir, name string, data []uint64) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := binary.Write(w, binary.LittleEndian, data); err != nil {
		return err
	}
	return w.Flush()
}

// writeUint32Pairs 顺序写入两个等长切片，避免拼接拷贝。
func writeUint32Pairs(dir, name string, a, b []uint32) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := binary.Write(w, binary.LittleEndian, a); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, b); err != nil {
		return err
	}
	return w.Flush()
}
