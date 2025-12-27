package processors

import (
	"awning-backend/common"
	"awning-backend/services"
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

type ImageProcessor struct {
	logger *slog.Logger
	cfg    *common.Config
	svc    *services.UnsplashService
}

func NewImageProcessor(cfg *common.Config, svc *services.UnsplashService) *ImageProcessor {
	logger := slog.With("processor", "ImageProcessor")

	return &ImageProcessor{
		logger: logger,
		cfg:    cfg,
		svc:    svc,
	}
}

// ProcessImageQuery processes an image query and returns results
func (p *ImageProcessor) ProcessImageQuery(ctx context.Context, query string, page, perPage int, orientation, orderBy string) (*services.UnsplashSearchResponse, error) {
	p.logger.Info("Processing image query", "query", query)

	results, err := p.svc.SearchPhotos(ctx, query, page, perPage, orientation, orderBy)
	if err != nil {
		p.logger.Error("Failed to search photos", "error", err)
		return nil, err
	}

	p.logger.Info("Image query processed successfully", "total_results", results.Total)

	return results, nil
}

type ImageProcessorNodeFilter func(*html.Node) bool
type ImageProcessorNodeWalker func(node *html.Node, parentKeywords ...string) []string

func (p *ImageProcessor) walkNodes(n *html.Node, filter ImageProcessorNodeFilter, walker ImageProcessorNodeWalker, parentKeywords ...string) {
	keywords := []string{}

	if filter(n) {
		kw := walker(n)

		if len(keywords) > 0 && len(kw) > 0 {
			keywords = append(keywords, kw...)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.walkNodes(c, filter, walker, keywords...)
	}
}

func (p *ImageProcessor) processDivWithBackgroundNode(_ context.Context, queryMap map[string]*ImageQueryRequest, n *html.Node, parentKeywords ...string) error {
	imgKeywords := p.getImageKeywords(n)

	if len(imgKeywords) > 0 {
		p.logger.Info("Found div/section with image keywords", "tag", n.Data, "image_keywords", imgKeywords)
	}

	if len(imgKeywords) == 0 {
		p.logger.Warn("No image keywords found for div/section node, skipping")
		return nil
	}

	hasImgChildren := hasAnyOfChildren(n, []string{"img"})
	if hasImgChildren {
		// The walker will handle this case by processing img children
		// with the parent keywords prepended
		return nil
	}

	// Assign background image to the div/section itself
	keywords := strings.Join(imgKeywords, ", ")

	query := &ImageQueryRequest{
		ID:       common.RandomID(),
		Type:     ImageQueryRequestTypeCssBackground,
		Node:     n,
		Keywords: keywords,
	}

	queryMap[query.ID] = query

	return nil
}

func (p *ImageProcessor) processImgNode(_ context.Context, queryMap map[string]*ImageQueryRequest, n *html.Node, keywords string) error {
	imgKeywords := p.getImageKeywords(n)

	if len(imgKeywords) == 0 {
		p.logger.Warn("No image keywords found for img node, skipping")
		return nil
	}

	keywords = strings.Join(imgKeywords, ", ")

	p.logger.Info("Found img node with image keywords", "tag", n.Data, "image_keywords", imgKeywords)

	// Search for images using the keywords
	query := &ImageQueryRequest{
		ID:       common.RandomID(),
		Type:     ImageQueryRequestTypeImgSrc,
		Node:     n,
		Keywords: keywords,
	}

	queryMap[query.ID] = query

	return nil
}

// func (p *ImageProcessor) processNode(ctx context.Context, queryMap map[string]*ImageQueryRequest, n *html.Node, keywords string) error {
// 	// p.logger.Info("Processing node", "tag", n.Data, "keywords", keywords)

// 	// If it's a div or section node, look for data-image-keywords attribute
// 	if n.Type == html.ElementNode && (n.Data == "div" || n.Data == "section") {

// 		attrs := []string{}
// 		for _, attr := range n.Attr {
// 			attrs = append(attrs, attr.Key+"="+attr.Val)
// 		}
// 		p.logger.Info("Processing div/section node", "tag", n.Data, "attrs", strings.Join(attrs, ","))

// 		imgKeywords := p.getImageKeywords(n)

// 		if len(imgKeywords) > 0 {
// 			p.logger.Info("Found div/section with image keywords", "tag", n.Data, "image_keywords", imgKeywords)
// 		}

// 		if len(imgKeywords) > 0 {

// 			hasImgChildren := hasAnyOfChildren(n, []string{"img"})
// 			if hasImgChildren {

// 				// Use the keywords for all img children
// 				for c := n.FirstChild; c != nil; c = c.NextSibling {
// 					if c.Type == html.ElementNode && c.Data == "img" {
// 						p.processNode(ctx, queryMap, c, keywords)
// 					}
// 				}

// 			} else {

// 				// Otherwise assign background image to the div/section itself
// 				keywords = strings.Join(imgKeywords, ", ")

// 				query := &ImageQueryRequest{
// 					ID:       common.RandomID(),
// 					Type:     ImageQueryRequestTypeCssBackground,
// 					Node:     n,
// 					Keywords: keywords,
// 				}

// 				queryMap[query.ID] = query
// 			}
// 		}

// 		return nil
// 	}

// 	// If it's an img node, check for data-image-keywords attribute
// 	if n.Type == html.ElementNode && n.Data == "img" {
// 		// imgKeywords := getAttr(n, "data-image-keywords")
// 		// if imgKeywords != "" {
// 		// 	keywords = imgKeywords
// 		// }

// 		// if keywords == "" {
// 		// 	p.logger.Warn("No keywords found for img node, skipping")
// 		// 	return nil
// 		// }

// 		imgKeywords := p.getImageKeywords(n)

// 		if len(imgKeywords) == 0 {
// 			p.logger.Warn("No image keywords found for img node, skipping")
// 			return nil
// 		}

// 		keywords = strings.Join(imgKeywords, ", ")

// 		p.logger.Info("Found img node with image keywords", "tag", n.Data, "image_keywords", imgKeywords)

// 		// Search for images using the keywords
// 		query := &ImageQueryRequest{
// 			ID:       common.RandomID(),
// 			Type:     ImageQueryRequestTypeImgSrc,
// 			Node:     n,
// 			Keywords: keywords,
// 		}

// 		queryMap[query.ID] = query

// 		return nil
// 	}

// 	// Otherwise, if it has children, recurse
// 	for c := n.FirstChild; c != nil; c = c.NextSibling {
// 		p.processNode(ctx, queryMap, c, "")
// 	}

// 	return nil
// }

func (h *ImageProcessor) Name() string {
	return "ImageProcessor"
}

var keywordsAttrs = []string{"data-image-keywords", "data-image-background-keywords", "title", "alt"}

func (h *ImageProcessor) getImageKeywords(n *html.Node) []string {
	keywordSet := make(map[string]struct{})

	for _, attrKey := range keywordsAttrs {
		attrVal := getAttr(n, attrKey)
		if attrVal != "" {
			parts := strings.Split(attrVal, ",")
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					keywordSet[trimmed] = struct{}{}
				}
			}
		}
	}

	var keywords []string
	for k := range keywordSet {
		keywords = append(keywords, k)
	}

	return keywords
}

func (h *ImageProcessor) Process(ctx context.Context, input []byte) ([]byte, error) {
	h.logger.Info("Processing images")

	sr := bytes.NewReader(input)

	// Use html dom to parse input
	rootNode, err := html.Parse(sr)
	if err != nil {
		h.logger.Error("Failed to parse HTML", "error", err)
		return nil, err
	}

	var queryMap = make(map[string]*ImageQueryRequest)

	queryReqs := make(chan *ImageQueryRequest)
	queryResp := make(chan *ImageQueryResult)

	asyncProcessor := NewAsyncImageProcessor(queryReqs, queryResp, h.svc)

	// process := func(n *html.Node, keywords string) {

	// 	// If it's an img node, check for data-image-keywords attribute
	// 	if n.Type == html.ElementNode && n.Data == "img" {
	// 		for _, attr := range n.Attr {
	// 			if attr.Key == "data-image-keywords" {
	// 				keywords := attr.Val
	// 				id := common.RandomID()
	// 				query := &ImageQueryRequest{
	// 					ID:       id,
	// 					Node:     n,
	// 					Keywords: keywords,
	// 				}
	// 				queryMap[id] = query
	// 				break
	// 			}
	// 		}
	// 	}
	// }

	// // Find data-image-keywords attribute
	// var f func(*html.Node)
	// f = func(n *html.Node) {
	// 	for c := n.FirstChild; c != nil; c = c.NextSibling {
	// 		// If it's an img[data-image-keywords], process it
	// 		if c.Type == html.ElementNode && c.Data == "img" || c.Data == "div" {
	// 			for _, attr := range c.Attr {
	// 				if attr.Key == "data-image-keywords" {
	// 					process(c)
	// 					break
	// 				}
	// 			}
	// 			// If we have children, recurse
	// 		} else if c.FirstChild != nil {
	// 			f(c)
	// 		}
	// 	}
	// }

	// h.processNode(ctx, queryMap, rootNode, "")

	// Filter: img or div/section nodes with data-image-keywords attribute
	filter := func(n *html.Node) bool {
		if n.Type == html.ElementNode && (n.Data == "img" || n.Data == "div" || n.Data == "section") {
			imgKeywords := h.getImageKeywords(n)
			return len(imgKeywords) > 0
		}
		return false
	}

	walker := func(n *html.Node, parentKeywords ...string) []string {
		// Walker: process the node

		imgKeywords := h.getImageKeywords(n)

		allKeywords := append(parentKeywords, imgKeywords...)

		keywords := strings.Join(allKeywords, ", ")

		if n.Data == "img" {
			h.processImgNode(ctx, queryMap, n, keywords)
		} else if n.Data == "div" || n.Data == "section" {
			h.processDivWithBackgroundNode(ctx, queryMap, n, keywords)
		}

		return imgKeywords
	}

	h.walkNodes(rootNode, filter, walker)

	if len(queryMap) == 0 {
		h.logger.Warn("No image queries found, returning original input")
		return input, nil
	}

	var imgResps = make(map[string]*ImageQueryResult, len(queryMap))
	var cssResps = make(map[string]*ImageQueryResult, len(queryMap))

	// Start async processor
	asyncProcessor.Start(ctx)

	// // Send queries
	// for _, req := range queryMap {
	// 	h.logger.Info("Sending image query request", "keywords", req.Keywords)
	// 	queryReqs <- req
	// }

	remaining := len(queryMap)

	wg := sync.WaitGroup{}

	wg.Add(1)

	// Collect responses
	go func() {
		defer wg.Done()

		h.logger.Info("Waiting for image query responses", "expected_count", remaining)

		for remaining > 0 {
			h.logger.Info("Waiting for next image query response", "remaining", remaining)

			resp := <-queryResp

			if resp == nil {
				h.logger.Warn("Received nil image query response")
				continue
			}

			req, exists := queryMap[resp.RequestID]
			if !exists {
				h.logger.Warn("No matching request found for response", "request_id", resp.RequestID)
				continue
			}

			switch req.Type {
			case ImageQueryRequestTypeImgSrc:
				imgResps[resp.RequestID] = resp
			case ImageQueryRequestTypeCssBackground:
				cssResps[resp.RequestID] = resp
			default:
				h.logger.Warn("Unknown image query request type", "type", req.Type)
			}

			h.logger.Info("Received image query response", "type", req.Type, "keywords", resp.Keywords, "image_count", len(resp.ImageURLs))
			remaining--
		}
	}()

	// Send queries
	for _, req := range queryMap {
		h.logger.Info("Sending image query request", "keywords", req.Keywords)
		queryReqs <- req
	}

	// Wait for all responses to be processed
	wg.Wait()
	close(queryResp)

	if remaining > 0 {
		h.logger.Warn("Some image queries did not return results", "remaining", remaining)
	}

	// Update the corresponding img node with the first image URL
	for _, resp := range imgResps {
		h.logger.Info("Updating img src for keywords", "keywords", resp.Keywords, "image_count", len(resp.ImageURLs))

		// Find the original request
		if resp == nil {
			h.logger.Warn("Received nil response for request ID", "request_id", resp.RequestID)
			continue
		}

		// Update the corresponding img node with the first image URL
		if resp == nil || len(resp.ImageURLs) == 0 {
			h.logger.Warn("No images found for request", "request_id", resp.RequestID, "keywords", resp.Keywords)
			continue
		}

		req, exists := queryMap[resp.RequestID]
		if !exists {
			h.logger.Warn("No matching request found for response", "request_id", resp.RequestID)
			continue
		}

		imageURL := resp.ImageURLs[0]
		h.logger.Info("Updating img src", "keywords", resp.Keywords, "image_url", imageURL)

		// Update src attribute
		for j, attr := range req.Node.Attr {
			if attr.Key == "src" {
				req.Node.Attr[j].Val = imageURL
				break
			}
		}
	}

	// Find the head node
	var headNode *html.Node
	var findHead func(*html.Node)
	findHead = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "head" {
			headNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findHead(c)
			if headNode != nil {
				return
			}
		}
	}
	findHead(rootNode)

	if headNode == nil {
		h.logger.Warn("No head node found in HTML, cannot insert CSS styles")
	} else {
		// Create style element
		styleNode := &html.Node{
			Type: html.ElementNode,
			Data: "style",
		}
		styleContent := &strings.Builder{}

		// Add CSS rules for background images
		for _, resp := range cssResps {
			h.logger.Info("Adding CSS background image for keywords", "keywords", resp.Keywords, "image_count", len(resp.ImageURLs))
			if resp == nil || len(resp.ImageURLs) == 0 {
				h.logger.Warn("No images found for CSS background request", "request_id", resp.RequestID, "keywords", resp.Keywords)
				continue
			}

			req, exists := queryMap[resp.RequestID]
			if !exists {
				h.logger.Warn("No matching request found for CSS background response", "request_id", resp.RequestID)
				continue
			}

			imageURL := resp.ImageURLs[0]
			h.logger.Info("Adding CSS rule", "keywords", resp.Keywords, "image_url", imageURL)

			// Create a unique class name
			className := "img-bg-" + resp.RequestID

			existingClass := getAttr(req.Node, "class")
			if existingClass != "" {
				className = existingClass + " " + className
			}

			// Add class to the node
			setAttr(req.Node, "class", className)

			setAttr(req.Node, "data-image-src", imageURL)

			// Add CSS rule to style content
			styleContent.WriteString("." + className + " {\n")
			styleContent.WriteString("  background-image: url('" + imageURL + "');\n")
			styleContent.WriteString("  background-size: cover;\n")
			styleContent.WriteString("  background-position: center;\n")
			styleContent.WriteString("}\n")
		}

		// Set the style content
		styleNode.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: styleContent.String(),
		})

		// Append the style node to the head
		headNode.AppendChild(styleNode)
	}

	// Serialize the updated HTML back to bytes
	var buf bytes.Buffer
	if err := html.Render(&buf, rootNode); err != nil {
		h.logger.Error("Failed to render updated HTML", "error", err)
		return nil, err
	}

	input = buf.Bytes()
	h.logger.Info("Image processing complete, returning updated input")
	// Return the updated input with processed images
	if len(input) == 0 {
		h.logger.Warn("Processed input is empty, returning original input")
		return input, nil
	}

	h.logger.Info("Returning processed input with images")
	return input, nil
}

type ImageQueryRequestType string

const (
	ImageQueryRequestTypeImgSrc        ImageQueryRequestType = "img_src"
	ImageQueryRequestTypeCssBackground ImageQueryRequestType = "css_background"
)

type ImageQueryRequest struct {
	ID       string
	Type     ImageQueryRequestType
	Node     *html.Node
	Keywords string
}

type ImageQueryResult struct {
	RequestID string
	Keywords  string
	ImageURLs []string
}

type AsyncImageProcessor struct {
	logger    *slog.Logger
	queryReqs chan *ImageQueryRequest
	queryResp chan *ImageQueryResult
	svc       *services.UnsplashService
}

func NewAsyncImageProcessor(
	queryReqs chan *ImageQueryRequest,
	queryResp chan *ImageQueryResult,
	svc *services.UnsplashService,
) *AsyncImageProcessor {
	logger := slog.With("processor", "AsyncImageProcessor")

	return &AsyncImageProcessor{
		logger:    logger,
		queryReqs: queryReqs,
		queryResp: queryResp,
		svc:       svc,
	}
}

func (p *AsyncImageProcessor) Start(bgCtx context.Context) {
	p.logger.Info("Starting async image processor")

	go func() {
		for {
			select {
			case req := <-p.queryReqs:
				p.logger.Info("Received image query request", "keywords", req.Keywords)

				// Process the image query
				results, err := p.svc.SearchPhotos(bgCtx, req.Keywords, 1, 5, "", "relevant")
				if err != nil {
					p.logger.Error("Failed to search photos", "error", err)
					continue
				}

				var imageURLs []string
				for _, photo := range results.Results {
					imageURLs = append(imageURLs, photo.URLs.Regular)
				}

				resp := &ImageQueryResult{
					RequestID: req.ID,
					Keywords:  req.Keywords,
					ImageURLs: imageURLs,
				}

				// Send the response
				p.logger.Info("Sending image query response to ImageProcessor", "keywords", req.Keywords, "image_count", len(imageURLs))
				p.queryResp <- resp

			case <-bgCtx.Done():
				p.logger.Info("Shutting down async image processor")
				return
			}
		}
	}()
}
