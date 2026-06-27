package pipeline

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/zeroedin/alloy/internal/cascade"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/plugin"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// PipelineState holds shared state initialized once per build.
// Used by both Build() and BuildIncremental() to avoid duplicating setup.
type PipelineState struct {
	Engine      tmpl.TemplateEngine
	Registry    *plugin.Registry
	Hooks       *plugin.HookRegistry
	CascadeData map[string]map[string]interface{}
	SiteData    map[string]interface{}
	ContentDir  string
	ContentBase string
}

// DiscoverPlugins creates a plugin registry and hook system, discovers
// plugins on disk, and loads them into the hook registry.
// Returns warnings for the caller to log (respects --quiet).
func DiscoverPlugins(cfg *config.Config) (*plugin.Registry, *plugin.HookRegistry, []string) {
	hooks := plugin.NewHookRegistry()
	hooks.SetTimeout(cfg.Plugins.Timeout)
	pluginsDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Plugins)
	registry := plugin.NewRegistry(pluginsDir)
	registry.SetPluginsDirRel(cfg.Structure.Plugins)
	if cfg.ProjectRoot != "" {
		registry.SetProjectRoot(cfg.ProjectRoot)
		registry.SetWASMCacheDir(resolveDir(cfg.ProjectRoot, ".alloy/wasm-cache"))
	}
	var warnings []string
	if err := registry.DiscoverPlugins(); err != nil {
		warnings = append(warnings, fmt.Sprintf("plugin discovery: %v", err))
	}
	for _, w := range registry.LoadPlugins(hooks) {
		warnings = append(warnings, w)
	}
	return registry, hooks, warnings
}

// InitPipelineState creates the template engine with plugin extensions,
// loads cascade and site data. Shared by Build() and BuildIncremental().
func InitPipelineState(cfg *config.Config, registry *plugin.Registry, hooks *plugin.HookRegistry) (*PipelineState, error) {
	engine, err := createEngine(cfg)
	if err != nil {
		return nil, err
	}
	tmpl.RegisterAssetFilters(cfg.ProjectRoot, cfg.Structure.Static, cfg.Structure.Assets, cfg.Structure.Content)
	if err := registerPluginExtensions(registry, engine); err != nil {
		return nil, err
	}
	if setter, ok := engine.(interface{ SetIncludesDir(string) }); ok {
		setter.SetIncludesDir(resolveDir(cfg.ProjectRoot, cfg.Structure.Layouts))
	}

	contentDir := resolveDir(cfg.ProjectRoot, cfg.Structure.Content)
	cascadeData, cascadeErr := cascade.LoadDirectoryCascade(contentDir)
	if cascadeErr != nil {
		log.Printf("warning: loading cascade data: %v", cascadeErr)
	}

	siteData, siteDataErr := loadSiteData(cfg)
	if siteDataErr != nil {
		return nil, siteDataErr
	}

	return &PipelineState{
		Engine:      engine,
		Registry:    registry,
		Hooks:       hooks,
		CascadeData: cascadeData,
		SiteData:    siteData,
		ContentDir:  contentDir,
		ContentBase: filepath.Base(contentDir),
	}, nil
}

func createEngine(cfg *config.Config) (tmpl.TemplateEngine, error) {
	var engine tmpl.TemplateEngine
	if cfg.Templates.Engine == "gotemplate" {
		engine = tmpl.NewGoEngine()
	} else {
		engine = tmpl.NewLiquidEngine()
	}
	tmpl.InitMarkdownify(content.MarkdownOptions{
		Unsafe:         cfg.Content.Markdown.Goldmark.UnsafeValue(),
		Typographer:    cfg.Content.Markdown.Goldmark.Typographer,
		AutoHeadingID:  cfg.Content.Markdown.Goldmark.AutoHeadingIDValue(),
		CustomElements: cfg.Content.Markdown.Goldmark.CustomElementsValue(),
	})
	if err := tmpl.RegisterBuiltinFilters(engine); err != nil {
		return nil, fmt.Errorf("registering template filters: %w", err)
	}
	tmpl.RegisterInlineTag(engine)
	return engine, nil
}

func registerPluginExtensions(registry *plugin.Registry, engine tmpl.TemplateEngine) error {
	for _, rt := range registry.Runtimes() {
		for _, filterName := range rt.RegisteredFilters() {
			name := filterName
			runtime := rt
			if err := engine.AddFilter(name, func(input interface{}, args ...interface{}) interface{} {
				result, err := runtime.CallFilter(name, input, args...)
				if err != nil {
					return input
				}
				return result
			}); err != nil {
				return fmt.Errorf("registering plugin filter %q: %w", name, err)
			}
		}
		for _, scName := range rt.RegisteredShortcodes() {
			name := scName
			runtime := rt
			if err := engine.AddTag(name, func(args []string, content string) string {
				result, err := runtime.CallShortcode(name, args, content)
				if err != nil {
					return ""
				}
				return result
			}); err != nil {
				return fmt.Errorf("registering plugin shortcode %q: %w", name, err)
			}
		}
	}
	return nil
}
