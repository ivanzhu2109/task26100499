package reuse

// Word 是文本中一个词的码点坐标区间 [Start, End)。
type Word struct {
	Start int
	End   int
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func lowerASCII(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// Tokenize 提取文本中所有英文词（最长连续 ASCII 字母串），
// 其他字符均为分隔符。返回词的码点坐标区间及对应的小写形式，
// 两者按出现顺序一一对应。
func Tokenize(text string) ([]Word, []string) {
	words, buf, offs := tokenizeLower(text)
	norms := make([]string, len(words))
	for i := range norms {
		norms[i] = string(buf[offs[i]:offs[i+1]])
	}
	return words, norms
}

// tokenizeLower 与 Tokenize 相同，但小写词集中存放在一个缓冲区中，
// 第 i 个词为 buf[offs[i]:offs[i+1]]，避免逐词分配字符串。
func tokenizeLower(text string) (words []Word, buf []byte, offs []int) {
	start := -1
	flush := func(end int) {
		if start >= 0 {
			words = append(words, Word{Start: start, End: end})
			offs = append(offs, len(buf))
			start = -1
		}
	}
	offs = append(offs, 0)
	pos := 0
	for _, r := range text {
		if isASCIILetter(r) {
			if start < 0 {
				start = pos
			}
			buf = append(buf, byte(lowerASCII(r)))
		} else {
			flush(pos)
		}
		pos++
	}
	flush(pos)
	return words, buf, offs
}
