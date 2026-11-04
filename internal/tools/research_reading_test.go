package tools

import (
	"strings"
	"testing"
)

func TestJEVReadingPagesAdvanceWithoutGaps(t *testing.T) {
	content := strings.Repeat("文", 4500) + "\r\n \t\r\n" + strings.Repeat("后", 8000)
	doc := newJEVDocument("https://example.com", content)
	if !doc.readNext("缺少的词") || doc.reading.ReadChars != 4506 || doc.reading.PagesRead != 1 || doc.reading.Complete {
		t.Fatalf("first page read=%d pages=%d complete=%v", doc.reading.ReadChars, doc.reading.PagesRead, doc.reading.Complete)
	}
	previous := doc.reading.ReadChars
	for doc.hasUnread() {
		if !doc.readNext("缺少的词") || doc.reading.ReadChars <= previous || doc.reading.ReadChars-previous > 4800 {
			t.Fatalf("page failed to advance: previous=%d now=%d", previous, doc.reading.ReadChars)
		}
		previous = doc.reading.ReadChars
		assertJEVPassageOffsets(t, doc, content)
	}
	if !doc.reading.Complete || doc.reading.Truncated || doc.reading.ReadChars != len([]rune(content)) || doc.readNext("again") {
		t.Fatalf("final reading = %+v", doc.reading)
	}
}

func TestJEVReadingLaterRelevantTextReplacesFallback(t *testing.T) {
	content := strings.Repeat("无关内容。", 1100) + "\n\n退款政策规定：七天内可申请退款。\n\n其他内容。"
	doc := newJEVDocument("https://example.com", content)
	doc.readNext("退款政策")
	if strings.Contains(doc.excerpt(), "七天") {
		t.Fatal("excerpt included unread text")
	}
	doc.readNext("退款政策")
	if !strings.Contains(doc.excerpt(), "七天内可申请退款") || strings.Contains(doc.excerpt(), "无关内容") {
		t.Fatal("later relevant paragraph did not replace fallback")
	}
	assertJEVPassageOffsets(t, doc, content)
}

func TestJEVReadingKeepsEarlierRelevantEvidence(t *testing.T) {
	first := "Refunds are available within seven days.\n\n"
	last := "Refunds for damaged items have a separate shipping policy.\n\n"
	content := first + strings.Repeat("unrelated text ", 400) + "\n\n" + last
	doc := newJEVDocument("https://example.com", content)
	doc.readNext("refunds damaged shipping policy")
	if !strings.Contains(doc.excerpt(), "seven days") {
		t.Fatal("early relevant paragraph not selected")
	}
	for doc.hasUnread() {
		doc.readNext("refunds damaged shipping policy")
	}
	if !strings.Contains(doc.excerpt(), "seven days") || !strings.Contains(doc.excerpt(), "damaged items") {
		t.Fatal("new evidence displaced all earlier relevant evidence")
	}
	assertJEVPassageOffsets(t, doc, content)
}

func TestJEVReadingRelevantLongParagraphAndExcerptLimit(t *testing.T) {
	content := strings.Repeat("甲", 2500) + "价格费用预算" + strings.Repeat("乙", 7000)
	doc := newJEVDocument("https://example.com", content)
	for doc.hasUnread() {
		doc.readNext("价格费用预算")
		if !strings.Contains(doc.excerpt(), "价格费用预算") {
			t.Fatal("match in middle of long paragraph lost")
		}
		assertJEVPassageOffsets(t, doc, content)
	}
	if doc.reading.Passages[0].Start == 0 {
		t.Fatal("long paragraph was truncated from its irrelevant beginning")
	}
	content = strings.Repeat("relevant "+strings.Repeat("x", 690)+"\n\n", 20)
	doc = newJEVDocument("https://example.com", content)
	for doc.hasUnread() {
		doc.readNext("relevant")
		assertJEVPassageOffsets(t, doc, content)
	}
	if len(doc.reading.Passages) < 2 {
		t.Fatal("excerpt budget unnecessarily limited to one paragraph")
	}
}

func TestJEVReadingRetentionLimitAndEmptyBody(t *testing.T) {
	content := strings.Repeat("字", 76000)
	doc := newJEVDocument("https://example.com", content)
	for doc.hasUnread() {
		doc.readNext("missing")
		if doc.reading.PagesRead > 30 {
			t.Fatal("pagination failed to terminate")
		}
	}
	if len(doc.body) != 75000 || doc.reading.ContentChars != 76000 || doc.reading.ReadChars != 75000 || !doc.reading.Truncated || doc.reading.Complete {
		t.Fatalf("truncated reading metadata = %+v", doc.reading)
	}
	assertJEVPassageOffsets(t, doc, content)
	for _, text := range []string{"", "短", "短文\n\n"} {
		doc := newJEVDocument("https://example.com", text)
		doc.readNext("unknown")
		if !doc.reading.Complete || doc.reading.Truncated || doc.hasUnread() || doc.excerpt() != text {
			t.Fatalf("short body reading incorrect: %+v", doc.reading)
		}
	}
}

func TestJEVReadingKeepsContextAroundWindowBoundary(t *testing.T) {
	content := strings.Repeat("x", 790) + " refunds" + strings.Repeat(" ", 20) + "are unavailable after seven days." + strings.Repeat("y", 900)
	doc := newJEVDocument("https://example.com", content)
	doc.readNext("refunds")
	if !strings.Contains(doc.excerpt(), "are unavailable after seven days") {
		t.Fatal("excerpt lost nearby qualification at window boundary")
	}
	assertJEVPassageOffsets(t, doc, content)
}

func TestJEVReadingHeadingRetainsInstallationSteps(t *testing.T) {
	content := "Installation\n\nRun npm install package-name."
	doc := newJEVDocument("https://example.com", content)
	doc.readNext("Installation")
	if !strings.Contains(doc.excerpt(), "Run npm install package-name.") {
		t.Fatal("heading excerpt lost installation steps")
	}
	assertJEVPassageOffsets(t, doc, content)
}

func TestJEVReadingHeadingRetainsRefundLimits(t *testing.T) {
	content := "退款政策\n\n签收后七天内可申请。\n\n逾期不受理，质量问题除外。"
	doc := newJEVDocument("https://example.com", content)
	doc.readNext("退款政策")
	for _, want := range []string{"七天内可申请", "逾期不受理，质量问题除外"} {
		if !strings.Contains(doc.excerpt(), want) {
			t.Fatalf("heading excerpt lost %q", want)
		}
	}
	assertJEVPassageOffsets(t, doc, content)
}

func TestJEVReadingShortQueryFindsLaterEvidence(t *testing.T) {
	content := strings.Repeat("无关内容。", 1100) + "\n\nGo provides a compiled language and standard tooling."
	doc := newJEVDocument("https://example.com", content)
	for doc.hasUnread() {
		doc.readNext(" Go? ")
	}
	if !strings.Contains(doc.excerpt(), "Go provides") || strings.Contains(doc.excerpt(), "无关内容") {
		t.Fatal("short query failed to find later evidence")
	}
	assertJEVPassageOffsets(t, doc, content)
}

func assertJEVPassageOffsets(t *testing.T, doc *jevDocument, content string) {
	t.Helper()
	if len([]rune(doc.excerpt())) > 2000 {
		t.Fatalf("excerpt exceeded 2000 characters: %d", len([]rune(doc.excerpt())))
	}
	body := []rune(content)
	previous := 0
	for _, passage := range doc.reading.Passages {
		if passage.Start < previous || passage.Start < 0 || passage.End <= passage.Start || passage.End > doc.reading.ReadChars {
			t.Fatalf("invalid passage offsets: %+v", passage)
		}
		if passage.Text != string(body[passage.Start:passage.End]) {
			t.Fatal("passage text differs from its original source range")
		}
		previous = passage.End
	}
}
