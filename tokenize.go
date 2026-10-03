package reuse

// wordSpan 记录一个词在原文中的码点区间 [start, end)。
type wordSpan struct {
	start int
	end   int
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func toLowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// decodeRuneLen 返回 s 起始处 UTF-8 字符的字节长度（不校验合法性）。
func decodeRuneLen(s string) int {
	if len(s) == 0 {
		return 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return 1
	case b < 0xE0:
		return 2
	case b < 0xF0:
		return 3
	default:
		return 4
	}
}

// tokenize 将 UTF-8 纯文本切分为词（最长连续 ASCII 字母串），
// 返回每个词的码点区间。坐标从 0 开始、以 Unicode 码点计数、左闭右开；
// 其他字符（含多字节字符）均视为分隔符。
func tokenize(text string) []wordSpan {
	spans, _ := tokenizeLower(text)
	return spans
}

// tokenizeLower 在 tokenize 的基础上同时返回每个词的小写字节形式，
// 单次扫描完成，供构建期批量取词使用。
func tokenizeLower(text string) ([]wordSpan, [][]byte) {
	var spans []wordSpan
	var words [][]byte
	runePos := 0
	wordStart := -1
	var cur []byte
	flush := func() {
		spans = append(spans, wordSpan{start: wordStart, end: runePos})
		words = append(words, cur)
		cur = nil
		wordStart = -1
	}
	for i := 0; i < len(text); {
		b := text[i]
		if b < 0x80 {
			if isASCIILetter(b) {
				if wordStart < 0 {
					wordStart = runePos
				}
				cur = append(cur, toLowerASCII(b))
			} else if wordStart >= 0 {
				flush()
			}
			i++
		} else {
			if wordStart >= 0 {
				flush()
			}
			i += decodeRuneLen(text[i:])
		}
		runePos++
	}
	if wordStart >= 0 {
		flush()
	}
	return spans, words
}
