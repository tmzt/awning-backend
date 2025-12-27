package processors

import (
	"log/slog"
	"strings"

	"golang.org/x/net/html"
)

func hasAnyAttr(n *html.Node, keys []string) bool {
	for _, key := range keys {
		for _, attr := range n.Attr {
			if attr.Key == key {
				return true
			}
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, value string) {
	for i, attr := range n.Attr {
		if attr.Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

func hasAnyClassOrPrefix(n *html.Node, classes ...string) bool {
	classAttr := getAttr(n, "class")
	if classAttr == "" {
		return false
	}

	// Construct class set for quick lookup
	classSet := make(map[string]struct{})
	for _, cls := range classes {
		classSet[cls] = struct{}{}
	}

	classList := strings.Fields(classAttr)

	// Look for exact matches first
	for _, cls := range classList {
		if _, ok := classSet[cls]; ok {
			return true
		}
	}

	// Look for prefix matches
	for _, cls := range classList {
		for _, prefix := range classes {
			if strings.HasPrefix(cls, prefix) {
				return true
			}
		}
	}

	return false
}

func hasAnyOfChildren(n *html.Node, keys []string) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			for _, key := range keys {
				if c.Data == key {
					return true
				}
			}
		}
	}
	return false
}

type NodeFilter func(*html.Node) bool
type NodeWalker func(node *html.Node) (stop bool)

func WalkNodes(logger *slog.Logger, n *html.Node, filter NodeFilter, walker NodeWalker) {
	if filter(n) {
		stop := walker(n)
		if stop {
			return
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		WalkNodes(logger, child, filter, walker)
	}
}
