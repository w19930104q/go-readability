package readability

import (
	"fmt"

	"github.com/go-shiori/dom"
	"golang.org/x/net/html"
)

// RevisionView selects which variant of a document containing editorial
// revision marks (<ins> and <del>) is extracted. The zero value keeps
// the historical behaviour where revision marks are left untouched.
type RevisionView int

const (
	// RevisionViewNone is the default: revision marks are kept as-is and
	// the document is parsed unchanged.
	RevisionViewNone RevisionView = iota

	// RevisionViewCurrent extracts the current, revised text: every <del>
	// subtree is removed and every <ins> element is unwrapped so its
	// children take its place.
	RevisionViewCurrent

	// RevisionViewOriginal extracts the original text: every <ins>
	// subtree is removed and every <del> element is unwrapped.
	RevisionViewOriginal
)

// String returns the canonical string representation of the view.
func (v RevisionView) String() string {
	switch v {
	case RevisionViewCurrent:
		return "current"
	case RevisionViewOriginal:
		return "original"
	default:
		return "none"
	}
}

// ParseRevisionView converts a string into a RevisionView. An empty
// string (or "none") means RevisionViewNone, which preserves the default
// behaviour. "current" and "original" select the corresponding view.
func ParseRevisionView(s string) (RevisionView, error) {
	switch s {
	case "", "none":
		return RevisionViewNone, nil
	case "current":
		return RevisionViewCurrent, nil
	case "original":
		return RevisionViewOriginal, nil
	default:
		return RevisionViewNone, fmt.Errorf("invalid revision view %q: want \"current\" or \"original\"", s)
	}
}

// applyRevisionView rewrites the revision marks in doc according to v.
// It is a no-op for RevisionViewNone and unknown values.
//
// The rejected subtree is removed before the retained mark is unwrapped,
// so a mark of the opposite kind nested inside a rejected ancestor can
// never be revived: it disappears together with the whole subtree.
// Unwrapping promotes the element's children in place, which keeps the
// remaining node order, links, text nodes and original whitespace.
//
// Only the body is transformed (when present) so explicit metadata in
// the document head is left untouched.
func applyRevisionView(doc *html.Node, v RevisionView) {
	if v != RevisionViewCurrent && v != RevisionViewOriginal {
		return
	}

	var removeTag, unwrapTag string
	switch v {
	case RevisionViewCurrent:
		removeTag, unwrapTag = "del", "ins"
	case RevisionViewOriginal:
		removeTag, unwrapTag = "ins", "del"
	}

	// Work on the body so head metadata stays untouched. Documents
	// without a body (e.g. parsed fragments) fall back to the root.
	root := doc
	if bodies := dom.GetElementsByTagName(doc, "body"); len(bodies) > 0 {
		root = bodies[0]
	}

	// Snapshot the matching nodes first: the structural changes below
	// must not disturb the traversal. Nodes nested inside a removed
	// ancestor still point at that detached ancestor as their parent, so
	// connectedness against root is used to skip them.
	removed := dom.GetElementsByTagName(root, removeTag)
	for _, node := range removed {
		if !isConnectedTo(node, root) {
			continue
		}
		node.Parent.RemoveChild(node)
	}

	// Unwrap the retained marks in document order. Nested marks of the
	// same kind are handled successively as unwrapping the outer element
	// leaves the inner one connected in the same position.
	unwrapped := dom.GetElementsByTagName(root, unwrapTag)
	for _, node := range unwrapped {
		if !isConnectedTo(node, root) {
			continue
		}
		unwrapNode(node)
	}
}

// isConnectedTo reports whether node is still attached beneath ancestor.
func isConnectedTo(node, ancestor *html.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent == ancestor {
			return true
		}
	}
	return false
}

// unwrapNode replaces node with its children in place. The children keep
// their original order, and text nodes (including whitespace-only ones)
// are moved verbatim.
func unwrapNode(node *html.Node) {
	parent := node.Parent
	for node.FirstChild != nil {
		child := node.FirstChild
		node.RemoveChild(child)
		parent.InsertBefore(child, node)
	}
	parent.RemoveChild(node)
}
