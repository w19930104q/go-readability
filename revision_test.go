package readability

import (
	"strings"
	"testing"

	"github.com/go-shiori/dom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

const revisionLongText = "The quick brown fox jumps over the lazy dog while the committee reviews every single clause of the proposal with great care and patience."

// Revision snippets with deliberately different lengths so view-specific
// Length and excerpts are distinguishable.
const (
	revisionInsertText = "A freshly drafted replacement sentence that explains the revised policy in additional detail for every reader of the new edition."
	revisionDeleteText = "Old wording."
)

func revisionPage(body string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Revision Test Page</title>
<meta property="og:site_name" content="Example Site">
<meta name="author" content="Jane Reporter">
<meta name="description" content="A page used to test editorial revision views.">
</head>
<body>
<article>
<h1>Revision Test Page</h1>
` + body + `
</article>
</body>
</html>`
}

// revisionPageNoDescription is like revisionPage but without an explicit
// description meta, so the excerpt is derived from the article body.
func revisionPageNoDescription(body string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Revision Test Page</title>
<meta property="og:site_name" content="Example Site">
<meta name="author" content="Jane Reporter">
</head>
<body>
<article>
<h1>Revision Test Page</h1>
` + body + `
</article>
</body>
</html>`
}

func revisionParagraph(id string) string {
	return `<p id="` + id + `">` + revisionLongText + " " + revisionLongText + `</p>`
}

func parseRevisionDoc(t *testing.T, page string) *html.Node {
	t.Helper()
	doc, err := dom.Parse(strings.NewReader(page))
	require.NoError(t, err)
	return doc
}

func parseArticle(t *testing.T, p Parser, page string) Article {
	t.Helper()
	article, err := p.ParseDocument(parseRevisionDoc(t, page), fakeHostURL)
	require.NoError(t, err)
	return article
}

func countTags(node *html.Node, tag string) int {
	if node == nil {
		return 0
	}
	return len(dom.GetElementsByTagName(node, tag))
}

// TestRevisionViewTransform covers the raw transform itself: inline and
// block-level marks, nested same-kind marks, opposite marks buried inside
// removed ancestors, node order, links and original whitespace.
func TestRevisionViewTransform(t *testing.T) {
	page := revisionPage(`
<p id="inline">A <ins>new </ins>word and an <del>old </del>word stay
  <a href="/path?x=1&amp;y=2">linked <del>deleted</del><ins>inserted</ins></a>.</p>
<div id="block">
 <ins><p id="ins-block">` + revisionLongText + `</p></ins>
 <del><p id="del-block">` + revisionLongText + `</p></del>
</div>
<p id="nested-same">Outer <ins>mid <ins>inner</ins> end</ins> tail.</p>
<p id="nested-opposite"><del>removed <ins>must not revive</ins> gone</del> and <ins>kept <del>must not revive either</del> alive</ins>.</p>`)

	t.Run("current", func(t *testing.T) {
		doc := parseRevisionDoc(t, page)
		applyRevisionView(doc, RevisionViewCurrent)

		assert.Equal(t, 0, countTags(doc, "del"), "all del subtrees removed")
		assert.Equal(t, 0, countTags(doc, "ins"), "all ins elements unwrapped")
		assert.Equal(t, 1, countTags(doc, "a"), "links preserved")

		text := dom.TextContent(doc)
		assert.Contains(t, text, "A new word and an word stay")
		assert.Contains(t, text, "linked inserted")
		assert.NotContains(t, text, "old")
		assert.NotContains(t, text, "deleted")
		assert.NotContains(t, text, "must not revive")
		assert.Contains(t, text, "kept  alive")

		// The link itself, including its href, survives the unwrapping.
		link := dom.QuerySelector(doc, "a")
		require.NotNil(t, link)
		assert.Equal(t, "/path?x=1&y=2", dom.GetAttribute(link, "href"))
		assert.Equal(t, "linked inserted", strings.TrimSpace(dom.TextContent(link)))

		// Unwrapped block keeps its child paragraph and its order.
		block := dom.GetElementByID(doc, "block")
		require.NotNil(t, block)
		assert.NotNil(t, dom.GetElementByID(doc, "ins-block"))
		assert.Nil(t, dom.GetElementByID(doc, "del-block"))
		assert.Equal(t, "ins-block", dom.FirstElementChild(block).Attr[0].Val)

		// The transform is scoped to the body: head metadata stays intact.
		head := dom.GetElementsByTagName(doc, "head")[0]
		assert.Contains(t, dom.OuterHTML(head), `content="A page used to test editorial revision views."`)
		assert.Equal(t, 0, countTags(head, "ins"))
		assert.Equal(t, 0, countTags(head, "del"))

		// Whitespace text nodes are moved verbatim during unwrapping.
		assert.Contains(t, text, "tail.")
	})

	t.Run("original", func(t *testing.T) {
		doc := parseRevisionDoc(t, page)
		applyRevisionView(doc, RevisionViewOriginal)

		assert.Equal(t, 0, countTags(doc, "ins"))
		assert.Equal(t, 0, countTags(doc, "del"))

		text := dom.TextContent(doc)
		assert.Contains(t, text, "A word and an old word stay")
		assert.Contains(t, text, "linked deleted")
		assert.NotContains(t, text, "new")
		assert.NotContains(t, text, "inserted")
		assert.NotContains(t, text, "must not revive")
		assert.Contains(t, text, "removed  gone")
		assert.NotNil(t, dom.GetElementByID(doc, "del-block"))
		assert.Nil(t, dom.GetElementByID(doc, "ins-block"))
		head := dom.GetElementsByTagName(doc, "head")[0]
		assert.Equal(t, 0, countTags(head, "ins"))
		assert.Equal(t, 0, countTags(head, "del"))
		assert.Contains(t, dom.OuterHTML(head), "Example Site")
	})

	t.Run("none leaves document untouched", func(t *testing.T) {
		doc := parseRevisionDoc(t, page)
		before := dom.OuterHTML(doc)
		applyRevisionView(doc, RevisionViewNone)
		applyRevisionView(doc, RevisionView(42))
		assert.Equal(t, before, dom.OuterHTML(doc))
	})
}

// TestRevisionViewParsing checks that Content, TextContent and Length all
// follow the selected view and that the derived excerpt follows too.
func TestRevisionViewParsing(t *testing.T) {
	// The revision sits in the first paragraph so, without an explicit
	// description meta, the excerpt is derived from the selected view.
	body := `<p id="p1">Opening paragraph. <del>` + revisionDeleteText + `</del> <ins>` +
		revisionInsertText + `</ins> ` + revisionLongText + `</p>` +
		revisionParagraph("p2") + revisionParagraph("p3")
	page := revisionPageNoDescription(body)

	defaultArticle := parseArticle(t, NewParser(), page)
	currentArticle := parseArticle(t, Parser{RevisionView: RevisionViewCurrent}, page)
	originalArticle := parseArticle(t, Parser{RevisionView: RevisionViewOriginal}, page)

	assert.Contains(t, defaultArticle.Content, "<ins")
	assert.Contains(t, defaultArticle.Content, "<del")

	assert.NotContains(t, currentArticle.Content, "<ins")
	assert.NotContains(t, currentArticle.Content, "<del")
	assert.NotContains(t, originalArticle.Content, "<ins")
	assert.NotContains(t, originalArticle.Content, "<del")

	// Current keeps the long inserted sentence, original loses it and
	// keeps only the shared surrounding text.
	assert.Contains(t, currentArticle.TextContent, revisionInsertText)
	assert.NotContains(t, originalArticle.TextContent, revisionInsertText)
	assert.Contains(t, originalArticle.TextContent, revisionDeleteText)
	assert.Less(t, originalArticle.Length, currentArticle.Length)
	assert.Equal(t, charCount(currentArticle.TextContent), currentArticle.Length)
	assert.Equal(t, charCount(originalArticle.TextContent), originalArticle.Length)

	// Excerpt is derived from the article body, so it follows the view.
	assert.NotEmpty(t, currentArticle.Excerpt)
	assert.Contains(t, currentArticle.Excerpt, revisionInsertText)
	assert.NotContains(t, originalArticle.Excerpt, revisionInsertText)
	assert.NotEqual(t, currentArticle.Excerpt, originalArticle.Excerpt)
}

// TestRevisionViewCheck ensures excluded text cannot make an empty view
// pass Check nor steal the article candidate.
func TestRevisionViewCheck(t *testing.T) {
	// All readable text lives inside <del>, so the current view is empty.
	deletedBody := `<del><p id="gone">` + strings.Repeat(revisionLongText+" ", 8) + `</p></del>` +
		`<p id="short">tiny</p>`
	emptyCurrentPage := revisionPage(deletedBody)

	current := Parser{RevisionView: RevisionViewCurrent}
	original := Parser{RevisionView: RevisionViewOriginal}

	assert.False(t, current.CheckDocument(parseRevisionDoc(t, emptyCurrentPage)),
		"current view must not pass Check when all long text is deleted")
	assert.True(t, original.CheckDocument(parseRevisionDoc(t, emptyCurrentPage)),
		"original view keeps the deleted text and stays readable")

	// The empty view must not produce an article from the excluded block.
	emptyArticle := parseArticle(t, current, emptyCurrentPage)
	assert.NotContains(t, emptyArticle.TextContent, revisionLongText)
	assert.Equal(t, 0, countTags(emptyArticle.Node, "del"))

	// Conversely: a long inserted block is the candidate in current view
	// but invisible in original view.
	insertedBody := `<ins><p id="added">` + strings.Repeat(revisionLongText+" ", 8) + `</p></ins>` +
		`<p id="rest">tiny leftover</p>`
	insertedPage := revisionPage(insertedBody)

	assert.True(t, current.CheckDocument(parseRevisionDoc(t, insertedPage)))
	assert.False(t, original.CheckDocument(parseRevisionDoc(t, insertedPage)),
		"original view must not count inserted-only text")

	insertedArticle := parseArticle(t, current, insertedPage)
	assert.Contains(t, insertedArticle.TextContent, revisionLongText)
	assert.Equal(t, 0, countTags(insertedArticle.Node, "ins"))
}

// TestRevisionViewCandidateCompetition checks scoring under competing
// candidates where one candidate is fully removed by the selected view.
func TestRevisionViewCandidateCompetition(t *testing.T) {
	body := `<div id="winner">` +
		revisionParagraph("w1") + revisionParagraph("w2") +
		`</div>
<del id="loser"><div>` +
		strings.Repeat(`<p>`+revisionLongText+` `+revisionLongText+`</p>`, 6) +
		`</div></del>`
	page := revisionPage(body)

	current := Parser{RevisionView: RevisionViewCurrent}
	article := parseArticle(t, current, page)

	assert.NotContains(t, article.Content, "loser")
	assert.Equal(t, 0, countTags(article.Node, "del"))
	assert.Contains(t, article.Content, "winner")
	assert.True(t, current.CheckDocument(parseRevisionDoc(t, page)))
}

// TestRevisionViewCleaningAndRetry exercises the conditional cleaning and
// flag-retry path on a view-transformed document, asserting every retry
// consistently honours the view.
func TestRevisionViewCleaningAndRetry(t *testing.T) {
	body := `<div>` +
		`<form><input type="text" value="noise"></form>` +
		revisionParagraph("c1") + revisionParagraph("c2") +
		`<div class="share"><ins><p>` + revisionLongText + `</p></ins><del><p>` +
		revisionLongText + `</p></del></div>` +
		`</div>`
	page := revisionPage(body)

	for _, view := range []RevisionView{RevisionViewCurrent, RevisionViewOriginal} {
		ps := Parser{
			RevisionView:   view,
			CharThresholds: 5000, // force the full retry ladder
			NTopCandidates: 5,
		}
		article := parseArticle(t, ps, page)
		assert.NotContains(t, article.Content, "<ins", "view %s: no ins survives retries", view)
		assert.NotContains(t, article.Content, "<del", "view %s: no del survives retries", view)
		assert.NotContains(t, article.Content, "<form", "view %s: cleaning still runs", view)
		assert.NotContains(t, article.Content, "<input", "view %s: cleaning still runs", view)

		// A threshold above any reachable length exhausts every retry
		// flag, proving the full ladder (four grabArticle passes) ran on
		// the view-transformed tree. The recorded attempts are reset when
		// ParseDocument returns, so flag state is the observable signal.
		ps2 := Parser{
			RevisionView:   view,
			CharThresholds: 1_000_000,
			NTopCandidates: 5,
		}
		exhausted := parseArticle(t, ps2, page)
		assert.False(t, ps2.flags.stripUnlikelys, "view %s: retry ladder exhausted", view)
		assert.False(t, ps2.flags.useWeightClasses, "view %s: retry ladder exhausted", view)
		assert.False(t, ps2.flags.cleanConditionally, "view %s: retry ladder exhausted", view)
		assert.NotContains(t, exhausted.Content, "<ins")
		assert.NotContains(t, exhausted.Content, "<del")
	}
}

// TestRevisionViewParseEquivalence ensures Parse and ParseDocument return
// the same view-specific result.
func TestRevisionViewParseEquivalence(t *testing.T) {
	page := revisionPage(revisionParagraph("a") +
		`<p>` + revisionLongText + ` <del>old phrase</del> <ins>new phrase</ins></p>`)

	for _, view := range []RevisionView{RevisionViewCurrent, RevisionViewOriginal} {
		ps1 := Parser{RevisionView: view}
		fromReader, err := ps1.Parse(strings.NewReader(page), fakeHostURL)
		require.NoError(t, err)

		ps2 := Parser{RevisionView: view}
		fromDoc, err := ps2.ParseDocument(parseRevisionDoc(t, page), fakeHostURL)
		require.NoError(t, err)

		assert.Equal(t, fromDoc.Content, fromReader.Content, "view %s", view)
		assert.Equal(t, fromDoc.TextContent, fromReader.TextContent, "view %s", view)
		assert.Equal(t, fromDoc.Length, fromReader.Length, "view %s", view)
	}
}

// TestRevisionViewDoesNotMutateInput guarantees both Check and Parse work
// on internal copies, leaving the caller's DOM intact.
func TestRevisionViewDoesNotMutateInput(t *testing.T) {
	page := revisionPage(`<p>` + revisionLongText + ` <ins>kept</ins> <del>dropped</del></p>`)

	for _, view := range []RevisionView{RevisionViewCurrent, RevisionViewOriginal} {
		doc := parseRevisionDoc(t, page)
		before := dom.OuterHTML(doc)

		ps := Parser{RevisionView: view}
		_ = ps.CheckDocument(doc)
		assert.Equal(t, before, dom.OuterHTML(doc), "CheckDocument mutated input for view %s", view)

		_, err := ps.ParseDocument(doc, fakeHostURL)
		require.NoError(t, err)
		assert.Equal(t, before, dom.OuterHTML(doc), "ParseDocument mutated input for view %s", view)
		assert.NotZero(t, countTags(doc, "ins"))
		assert.NotZero(t, countTags(doc, "del"))
	}
}

// TestRevisionViewParserReuse verifies one Parser can be switched between
// views across calls without leaking state.
func TestRevisionViewParserReuse(t *testing.T) {
	page := revisionPage(revisionParagraph("r1") +
		`<p>` + revisionLongText + ` <del>` + revisionDeleteText + `</del><ins>` +
		revisionInsertText + `</ins></p>`)
	doc := parseRevisionDoc(t, page)

	ps := NewParser()
	first := parseArticle(t, ps, page) // default: marks kept

	ps.RevisionView = RevisionViewCurrent
	second := parseArticle(t, ps, page)

	ps.RevisionView = RevisionViewOriginal
	third := parseArticle(t, ps, page)

	ps.RevisionView = RevisionViewNone
	fourth := parseArticle(t, ps, page)

	assert.Contains(t, first.Content, "<ins")
	assert.NotContains(t, second.Content, "<ins")
	assert.NotContains(t, third.Content, "<ins")
	assert.Contains(t, fourth.Content, "<ins", "switching back restores default behaviour")

	assert.Greater(t, second.Length, third.Length)
	assert.Equal(t, first.Content, fourth.Content)

	// Reusing one parsed document across view switches is also safe.
	psCurrent := NewParserWithView(RevisionViewCurrent)
	fromSharedA, err := psCurrent.ParseDocument(doc, fakeHostURL)
	require.NoError(t, err)
	psOriginal := NewParserWithView(RevisionViewOriginal)
	fromSharedB, err := psOriginal.ParseDocument(doc, fakeHostURL)
	require.NoError(t, err)
	assert.NotContains(t, fromSharedA.Content, "<del")
	assert.NotContains(t, fromSharedB.Content, "<ins")
	assert.NotZero(t, countTags(doc, "ins"))
}

// NewParserWithView is a small local helper used by reuse tests.
func NewParserWithView(view RevisionView) Parser {
	ps := NewParser()
	ps.RevisionView = view
	return ps
}

// TestRevisionViewHeadMetadataUntouched confirms explicit head metadata
// is identical for every view while body-derived fields differ.
func TestRevisionViewHeadMetadataUntouched(t *testing.T) {
	page := revisionPage(revisionParagraph("m1") +
		`<p>` + revisionLongText + ` <del>` + revisionDeleteText + `</del><ins>` +
		revisionInsertText + `</ins></p>`)

	none := parseArticle(t, NewParser(), page)
	current := parseArticle(t, NewParserWithView(RevisionViewCurrent), page)
	original := parseArticle(t, NewParserWithView(RevisionViewOriginal), page)

	for _, got := range []Article{none, current, original} {
		assert.Equal(t, "Revision Test Page", got.Title)
		assert.Equal(t, "Example Site", got.SiteName)
		assert.Equal(t, "Jane Reporter", got.Byline)
		assert.Equal(t, "en", got.Language)
	}
	assert.NotEqual(t, current.Content, original.Content)
}

// TestRevisionViewNoMarksCompatibility checks pages without revision marks
// produce identical output in every mode, including legacy behaviour.
func TestRevisionViewNoMarksCompatibility(t *testing.T) {
	page := revisionPage(revisionParagraph("n1") + revisionParagraph("n2") + revisionParagraph("n3"))

	baseline := parseArticle(t, NewParser(), page)
	for _, view := range []RevisionView{RevisionViewCurrent, RevisionViewOriginal} {
		got := parseArticle(t, NewParserWithView(view), page)
		assert.Equal(t, baseline.Content, got.Content, "view %s", view)
		assert.Equal(t, baseline.TextContent, got.TextContent, "view %s", view)
		assert.Equal(t, baseline.Length, got.Length, "view %s", view)
		psView := NewParserWithView(view)
		assert.True(t, psView.CheckDocument(parseRevisionDoc(t, page)))
	}
}

// TestParseRevisionView covers the string conversion used by the CLI.
func TestParseRevisionView(t *testing.T) {
	cases := map[string]struct {
		view RevisionView
		err  bool
	}{
		"":         {RevisionViewNone, false},
		"none":     {RevisionViewNone, false},
		"current":  {RevisionViewCurrent, false},
		"original": {RevisionViewOriginal, false},
		"CURRENT":  {RevisionViewNone, true},
		"nope":     {RevisionViewNone, true},
	}
	for input, want := range cases {
		got, err := ParseRevisionView(input)
		if want.err {
			assert.Error(t, err, "input %q", input)
			continue
		}
		require.NoError(t, err, "input %q", input)
		assert.Equal(t, want.view, got, "input %q", input)
	}

	assert.Equal(t, "none", RevisionViewNone.String())
	assert.Equal(t, "current", RevisionViewCurrent.String())
	assert.Equal(t, "original", RevisionViewOriginal.String())
}
