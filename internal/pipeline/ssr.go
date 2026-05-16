package pipeline

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ssr"
)

// BuildPhase1 runs Phase 1 (content rendering) and returns a map of
// source paths to intermediate HTML. Custom element tags are preserved
// as raw tags — they are not rendered until Phase 2 SSR.
func BuildPhase1(cfg *config.Config) (map[string]string, error) {
	config.ApplyDefaults(cfg)

	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	pages, err := content.DiscoverWithFormats(contentDir, cfg.Content.Formats)
	if err != nil {
		return nil, fmt.Errorf("content discovery: %w", err)
	}

	pages = content.FilterByLifecycle(pages, time.Now(), cfg.IncludeDrafts)

	result := make(map[string]string, len(pages))

	mdOpts := content.MarkdownOptions{
		Unsafe:        cfg.Content.Markdown.Goldmark.UnsafeValue(),
		Typographer:   cfg.Content.Markdown.Goldmark.Typographer,
		TemplateTags:  cfg.Content.Markdown.Goldmark.TemplateTagsValue(),
		AutoHeadingID: cfg.Content.Markdown.Goldmark.AutoHeadingIDValue(),
	}
	md := content.CreateGoldmark(mdOpts)

	for _, page := range pages {
		html, _, err := content.RenderMarkdown(page.Body, md)
		if err != nil {
			return nil, fmt.Errorf("template rendering: %s: %w", page.RelPath, err)
		}
		result[page.RelPath] = string(html)
	}

	return result, nil
}

// BuildPhase2 runs Phase 2 (SSR transform) on the intermediate HTML
// from Phase 1. For each page with custom elements, pipes the full page
// HTML to the ssr.command via stdin and reads transformed HTML from stdout.
// Pages without custom elements pass through unchanged.
// Mode "exec" (default): one process per page.
// Mode "stream": persistent process with NUL-delimited messages.
func BuildPhase2(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	if ssrCfg == nil {
		return intermediateHTML, nil
	}

	if ssrCfg.Command == "" {
		return nil, fmt.Errorf("ssr.command is empty")
	}

	// Stream mode: use a persistent process
	if ssrCfg.Mode == "stream" {
		return buildPhase2Stream(intermediateHTML, ssrCfg)
	}

	// Exec mode (default): one process per page
	return buildPhase2Exec(intermediateHTML, ssrCfg)
}

func buildPhase2Exec(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	timeout := 30 * time.Second
	if ssrCfg.Timeout != "" {
		d, err := time.ParseDuration(ssrCfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid ssr.timeout %q: %w", ssrCfg.Timeout, err)
		}
		timeout = d
	}

	result := make(map[string]string, len(intermediateHTML))

	for path, html := range intermediateHTML {
		tags := ssr.ScanComponents(html)
		if len(tags) == 0 {
			result[path] = html
			continue
		}

		// Extract body content — only pipe the body to the SSR command,
		// preserve the document skeleton (DOCTYPE, head, scripts).
		body, before, after := ssr.ExtractBody(html)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		rendered, err := ssr.RenderPageWithTimeout(ctx, ssrCfg.Command, body)
		cancel()
		if err != nil {
			log.Printf("warning: SSR failed for %s: %v", path, err)
			result[path] = html
			continue
		}
		result[path] = ssr.ReassembleDocument(before, rendered, after)
	}

	return result, nil
}

func buildPhase2Stream(intermediateHTML map[string]string, ssrCfg *config.SSRConfig) (map[string]string, error) {
	timeout := 30 * time.Second
	if ssrCfg.Timeout != "" {
		d, err := time.ParseDuration(ssrCfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid ssr.timeout %q: %w", ssrCfg.Timeout, err)
		}
		timeout = d
	}

	sr, err := ssr.NewStreamRenderer(ssrCfg.Command)
	if err != nil {
		return nil, fmt.Errorf("ssr stream start %q: %w", ssrCfg.Command, err)
	}
	defer sr.Close()

	result := make(map[string]string, len(intermediateHTML))

	for path, html := range intermediateHTML {
		tags := ssr.ScanComponents(html)
		if len(tags) == 0 {
			result[path] = html
			continue
		}

		body, before, after := ssr.ExtractBody(html)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		rendered, err := sr.RenderPageWithTimeout(ctx, body)
		cancel()
		if err != nil {
			if restartErr := sr.Restart(); restartErr != nil {
				log.Printf("warning: SSR stream restart failed for %s: %v", path, restartErr)
				result[path] = html
				continue
			}
			retryCtx, retryCancel := context.WithTimeout(context.Background(), timeout)
			rendered, err = sr.RenderPageWithTimeout(retryCtx, body)
			retryCancel()
			if err != nil {
				log.Printf("warning: SSR stream failed after restart for %s: %v", path, err)
				result[path] = html
				continue
			}
		}
		result[path] = ssr.ReassembleDocument(before, rendered, after)
	}

	return result, nil
}
