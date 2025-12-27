package processors

import (
	"awning-backend/common"
	"bytes"
	"context"
	"log/slog"

	"golang.org/x/net/html"
)

const (
	TAILWIND_CDN_CSS_URL = "https://cdnjs.cloudflare.com/ajax/libs/tailwindcss/2.2.19/tailwind.min.css"
)

type HeaderProcessor struct {
	logger *slog.Logger
	cfg    *common.Config
}

func NewHeaderProcessor(cfg *common.Config) *HeaderProcessor {
	logger := slog.With("processor", "HeaderProcessor")

	return &HeaderProcessor{
		logger: logger,
		cfg:    cfg,
	}
}

func (p *HeaderProcessor) Name() string {
	return "HeaderProcessor"
}

func (p *HeaderProcessor) Process(ctx context.Context, input []byte) ([]byte, error) {
	p.logger.Info("Processing images")

	sr := bytes.NewReader(input)

	// Use html dom to parse input
	rootNode, err := html.Parse(sr)
	if err != nil {
		p.logger.Error("Failed to parse HTML", "error", err)
		return nil, err
	}

	p.logger.Info("Root node", "type", rootNode.Type, "data", rootNode.Data)

	htmlNode := rootNode.FirstChild
	for htmlNode != nil && !(htmlNode.Type == html.ElementNode && htmlNode.Data == "html") {
		p.logger.Info("Skipping node", "type", htmlNode.Type, "data", htmlNode.Data)
		htmlNode = htmlNode.NextSibling
	}

	if htmlNode == nil {
		p.logger.Warn("No <html> element found in HTML")
		return input, nil
	}

	head := htmlNode.FirstChild
	for head != nil && !(head.Type == html.ElementNode && head.Data == "head") {
		p.logger.Info("Skipping node", "type", head.Type, "data", head.Data)
		head = head.NextSibling
	}

	if head == nil {
		p.logger.Warn("No <head> element found in HTML")
		return input, nil
	}

	p.logger.Info("Adding Tailwind CSS link to <head> - not supported by Tailwind CDN for production use")

	// Add Tailwind CSS link
	linkNode := &html.Node{
		Type: html.ElementNode,
		Data: "link",
		Attr: []html.Attribute{
			{Key: "rel", Val: "stylesheet"},
			{Key: "href", Val: TAILWIND_CDN_CSS_URL},
		},
	}
	head.AppendChild(linkNode)

	var buf bytes.Buffer
	if err := html.Render(&buf, rootNode); err != nil {
		p.logger.Error("Failed to render modified HTML", "error", err)
		return nil, err
	}

	return buf.Bytes(), nil
}
