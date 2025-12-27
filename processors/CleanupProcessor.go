package processors

import (
	"awning-backend/common"
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/net/html"
)

// CleanupProcessor implements the Processor interface.
type CleanupProcessor struct {
	logger *slog.Logger
	cfg    *common.Config
}

func NewCleanupProcessor(cfg *common.Config) *CleanupProcessor {
	logger := slog.With("processor", "CleanupProcessor")

	return &CleanupProcessor{
		logger: logger,
		cfg:    cfg,
	}
}

func (c *CleanupProcessor) Name() string {
	return "CleanupProcessor"
}

func (c *CleanupProcessor) cleanupBrInGrids(rootNode *html.Node) {
	c.logger.Info("Cleaning up <br> tags in grid containers")

	filter := func(n *html.Node) bool {
		return true
	}

	walker := func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "br" {
			c.logger.Info("Found <br> tag")

			parent := n.Parent
			if parent != nil && hasAnyClassOrPrefix(parent, "grid", "grid-cols-", "grid-rows-") {
				c.logger.Info("Removing <br> tag inside grid container")
				// Remove the <br> node
				parent.RemoveChild(n)
				// Returning true to stop further processing of this node
				return true
			}
		}

		// Keep walking
		return false
	}

	WalkNodes(c.logger, rootNode, filter, walker)
}

// Process performs the cleanup operation.
func (c *CleanupProcessor) Process(_ context.Context, input []byte) ([]byte, error) {
	fmt.Println("Performing cleanup...")

	sr := bytes.NewReader(input)

	// Use html dom to parse input
	rootNode, err := html.Parse(sr)
	if err != nil {
		c.logger.Error("Failed to parse HTML", "error", err)
		return nil, err
	}

	c.cleanupBrInGrids(rootNode)

	// Render the modified HTML back to bytes
	var outputBuf bytes.Buffer
	if err := html.Render(&outputBuf, rootNode); err != nil {
		c.logger.Error("Failed to render HTML", "error", err)
		return nil, err
	}

	return outputBuf.Bytes(), nil
}
