package readability

import (
	"io"
	"math"
	"strings"

	"github.com/go-shiori/dom"
	"github.com/go-shiori/go-readability/internal/re2go"
	"golang.org/x/net/html"
)

// Check checks whether the input is readable without parsing the whole thing.
func (ps *Parser) Check(input io.Reader) bool {
	// Parse input
	doc, err := dom.Parse(input)
	if err != nil {
		return false
	}

	return ps.CheckDocument(doc)
}

// CheckDocument checks whether the document is readable without parsing the whole thing.
func (ps *Parser) CheckDocument(doc *html.Node) bool {
	// Resolve revision marks according to the selected view so the
	// readability check scores the same variant that Parse would extract.
	// A rejected revision may carry most of the text, so running the
	// check on the unfiltered tree could let a view that ends up empty
	// pass the readability gate. The document is only cloned when a view
	// is active, keeping the default behaviour allocation-free, and the
	// caller's DOM is never modified.
	workingDoc := doc
	if ps.RevisionView == RevisionViewCurrent || ps.RevisionView == RevisionViewOriginal {
		workingDoc = dom.Clone(doc, true)
		applyRevisionView(workingDoc, ps.RevisionView)
	}

	// Get <p> and <pre> nodes.
	nodes := dom.QuerySelectorAll(workingDoc, "p, pre, article")

	// Also get <div> nodes which have <br> node(s) and append
	// them into the `nodes` variable.
	// Some articles' DOM structures might look like :
	//
	// <div>
	//     Sentences<br>
	//     <br>
	//     Sentences<br>
	// </div>
	//
	// So we need to make sure only fetch the div once.
	// To do so, we will use map as dictionary.
	tracker := make(map[*html.Node]struct{})
	for _, br := range dom.QuerySelectorAll(workingDoc, "div > br") {
		if br.Parent == nil {
			continue
		}

		if _, exist := tracker[br.Parent]; !exist {
			tracker[br.Parent] = struct{}{}
			nodes = append(nodes, br.Parent)
		}
	}

	// This is a little cheeky, we use the accumulator 'score' to decide what
	// to return from this callback.
	score := float64(0)
	return ps.someNode(nodes, func(node *html.Node) bool {
		if !ps.isProbablyVisible(node) {
			return false
		}

		matchString := dom.ClassName(node) + " " + dom.ID(node)
		if re2go.IsUnlikelyCandidates(matchString) &&
			!re2go.MaybeItsACandidate(matchString) {
			return false
		}

		if dom.TagName(node) == "p" && ps.hasAncestorTag(node, "li", -1, nil) {
			return false
		}

		nodeText := strings.TrimSpace(dom.TextContent(node))
		nodeTextLength := len(nodeText)
		if nodeTextLength < 140 {
			return false
		}

		score += math.Sqrt(float64(nodeTextLength - 140))
		return score > 20
	})
}
