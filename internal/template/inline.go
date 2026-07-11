package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

var allowedExtensions = map[string]bool{
	".svg": true, ".html": true, ".htm": true, ".txt": true,
	".css": true, ".js": true, ".json": true, ".xml": true,
	".toml": true, ".yaml": true, ".yml": true, ".md": true,
}

var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".avif": true, ".ico": true, ".bmp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
	".eot": true, ".pdf": true, ".zip": true, ".tar": true,
	".gz": true, ".mp3": true, ".mp4": true, ".wav": true,
	".ogg": true, ".webm": true, ".mov": true,
}

// RegisterInlineTag registers the {% inline %} tag on the given engine.
// The tag reads a file relative to the content file's directory and
// inserts its raw contents without template processing.
func RegisterInlineTag(engine TemplateEngine) {
	le, ok := engine.(*liquidEngine)
	if !ok {
		return
	}
	le.env.RegisterTag("inline", tags.TagConstructor(
		func(tagName, markup string, parseContext liquid.ParseContextInterface) (interface{}, error) {
			return &inlineTag{
				Tag:    liquid.NewTag(tagName, markup, parseContext),
				markup: markup,
			}, nil
		},
	))
}

type inlineTag struct {
	*liquid.Tag
	markup string
}

func (t *inlineTag) Parse(tokenizer *liquid.Tokenizer) error { return nil }

func (t *inlineTag) Render(context liquid.TagContext) string {
	result, err := t.resolve(context)
	if err != nil {
		context.HandleError(liquid.NewInternalError(err.Error()), nil)
		return ""
	}
	return result
}

func (t *inlineTag) RenderToOutputBuffer(context liquid.TagContext, output *string) {
	*output += t.Render(context)
}

func (t *inlineTag) resolve(context liquid.TagContext) (string, error) {
	args := parseTagArgs(t.markup)
	if len(args) == 0 {
		return "", fmt.Errorf("inline tag requires a file path argument")
	}
	relPath := args[0]

	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("inline rejects absolute path: %s", relPath)
	}

	if !strings.HasPrefix(relPath, "./") && !strings.HasPrefix(relPath, "../") {
		return "", fmt.Errorf("inline path must start with ./ or ../: %s", relPath)
	}

	ext := strings.ToLower(filepath.Ext(relPath))
	if binaryExtensions[ext] {
		return "", fmt.Errorf("inline rejects binary file type %s: use <img> instead", ext)
	}
	if !allowedExtensions[ext] {
		return "", fmt.Errorf("inline rejects file type: %s is not in the text-type allowlist", ext)
	}

	contentDir, _ := context.FindVariable("_contentDir", false).(string)
	if contentDir == "" {
		return "", fmt.Errorf("inline tag requires _contentDir in render context")
	}

	resolved := filepath.Join(contentDir, relPath)
	resolved = filepath.Clean(resolved)

	contentRoot, _ := context.FindVariable("_contentRoot", false).(string)
	if contentRoot != "" {
		contentRoot = filepath.Clean(contentRoot)
		rel, err := filepath.Rel(contentRoot, resolved)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("inline path escapes content root: %s", relPath)
		}
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("inline file not found: %s: %w", relPath, err)
	}

	return string(data), nil
}
