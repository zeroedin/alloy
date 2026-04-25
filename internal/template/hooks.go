package template

import (
	"os"
	"path/filepath"
	"strings"
)

var validHookTypes = map[string]bool{
	"link":       true,
	"heading":    true,
	"image":      true,
	"codeblock":  true,
	"blockquote": true,
	"table":      true,
}

func DiscoverRenderHooks(layoutsDir, engine string) (map[string]string, error) {
	ext := ".liquid"
	if engine == "gotemplate" {
		ext = ".html"
	}

	markupDir := filepath.Join(layoutsDir, "_markup")
	entries, err := os.ReadDir(markupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	hooks := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ext) {
			continue
		}
		if !strings.HasPrefix(name, "render-") {
			continue
		}

		hookName := strings.TrimSuffix(strings.TrimPrefix(name, "render-"), ext)

		if !isValidHookName(hookName) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(markupDir, name))
		if err != nil {
			return nil, err
		}
		hooks[hookName] = string(data)
	}

	return hooks, nil
}

func isValidHookName(name string) bool {
	if validHookTypes[name] {
		return true
	}
	if strings.HasPrefix(name, "codeblock-") {
		return true
	}
	return false
}
