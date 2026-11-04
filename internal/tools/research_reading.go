package tools

import (
	"sort"
	"strings"
	"unicode"
)

// JEVPassage 的偏移按原正文 Unicode 字符计数，采用从零开始的半开区间。
type JEVPassage struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// JEVReading 分别记录阅读进度和摘录范围；读完正文不代表摘录包含全文。
type JEVReading struct {
	URL          string       `json:"url"`
	ContentChars int          `json:"content_chars"`
	ReadChars    int          `json:"read_chars"`
	PagesRead    int          `json:"pages_read"`
	Truncated    bool         `json:"truncated"`
	Complete     bool         `json:"complete"`
	Passages     []JEVPassage `json:"passages"`
}

type jevDocument struct {
	body    []rune
	reading JEVReading
}

func newJEVDocument(url, content string) *jevDocument {
	body := []rune(content)
	count := len(body)
	if count > 75000 {
		body = append([]rune(nil), body[:75000]...)
	}
	return &jevDocument{body: body, reading: JEVReading{
		URL: url, ContentChars: count, Complete: count == 0,
		Truncated: count > len(body), Passages: []JEVPassage{},
	}}
}

func (d *jevDocument) hasUnread() bool {
	return d.reading.ReadChars < len(d.body)
}

func (d *jevDocument) readNext(query string) bool {
	if !d.hasUnread() {
		return false
	}
	start := maxInt(0, d.reading.ReadChars-200)
	end := minInt(start+5000, len(d.body))
	if end < len(d.body) {
		// 页间重叠提供上下文；只在末尾 1,000 字内寻找段落边界，保证推进。
		for i := end - 1; i >= maxInt(start+4000, d.reading.ReadChars); i-- {
			if boundary := jevParagraphEnd(d.body, i); boundary > 0 && boundary <= end {
				end = boundary
				break
			}
		}
	}
	d.reading.ReadChars = end
	d.reading.PagesRead++
	d.reading.Complete = end == d.reading.ContentChars
	d.reading.Passages = jevRelevantPassages(d.body[:end], query)
	return true
}

func (d *jevDocument) excerpt() string {
	parts := make([]string, 0, len(d.reading.Passages))
	for _, passage := range d.reading.Passages {
		parts = append(parts, passage.Text)
	}
	return strings.Join(parts, "\n\n")
}

// jevParagraphEnd 同时识别 LF、CRLF 和含空白的空行。
func jevParagraphEnd(body []rune, i int) int {
	if body[i] != '\n' {
		return 0
	}
	j := i + 1
	for j < len(body) && (body[j] == ' ' || body[j] == '\t' || body[j] == '\r') {
		j++
	}
	if j < len(body) && body[j] == '\n' {
		return j + 1
	}
	return 0
}

func jevRelevantPassages(body []rune, query string) []JEVPassage {
	terms := tokenizeQuery(query)
	runes := []rune(query)
	for i := 1; i < len(runes); i++ {
		if unicode.Is(unicode.Han, runes[i-1]) && unicode.Is(unicode.Han, runes[i]) {
			terms = append(terms, string(runes[i-1:i+1]))
		}
	}
	terms = uniqueStrings(terms)
	if len(terms) == 0 {
		term := strings.TrimFunc(strings.ToLower(query), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
		if term != "" {
			terms = []string{term}
		}
	}
	type candidate struct{ start, end, score, context int }
	var candidates []candidate
	for start := 0; start < len(body); {
		end := start
		for end < len(body) {
			if boundary := jevParagraphEnd(body, end); boundary > 0 {
				end = boundary
				break
			}
			end++
		}
		// 长段落按带重叠的原文窗口排名，避免命中词被段首截断丢掉。
		for offset := start; offset < end; offset += 600 {
			limit := minInt(offset+800, end)
			text := strings.ToLower(string(body[offset:limit]))
			score := 0
			context := 0
			for _, term := range terms {
				if match := strings.Index(text, term); match >= 0 {
					score++
					position := len([]rune(text[:match]))
					context += minInt(position, limit-offset-position-len([]rune(term)))
				}
			}
			if score > 0 {
				// 命中标题或短段时带上后续说明，避免只保留标题而丢掉限定条件。
				if end-start < 800 {
					limit = minInt(offset+800, len(body))
				}
				candidates = append(candidates, candidate{offset, limit, score, context})
			}
			if limit == end {
				break
			}
		}
		start = end
	}
	if len(candidates) == 0 {
		end := minInt(2000, len(body))
		if end == 0 {
			return []JEVPassage{}
		}
		return []JEVPassage{{Start: 0, End: end, Text: string(body[:end])}}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].context > candidates[j].context
	})
	passages := []JEVPassage{}
	remaining := 2000
	for _, c := range candidates {
		overlaps := false
		for _, passage := range passages {
			if c.start < passage.End && c.end > passage.Start {
				overlaps = true
				break
			}
		}
		separator := 0
		if len(passages) > 0 {
			separator = 2
		}
		if overlaps || c.end-c.start+separator > remaining {
			continue
		}
		passages = append(passages, JEVPassage{Start: c.start, End: c.end, Text: string(body[c.start:c.end])})
		remaining -= c.end - c.start + separator
	}
	sort.Slice(passages, func(i, j int) bool { return passages[i].Start < passages[j].Start })
	return passages
}
