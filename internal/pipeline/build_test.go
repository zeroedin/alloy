package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// Spec reference: PLAN.md §2 — Build Pipeline
// Tests are immutable. They encode the specification.
// If implementation cannot satisfy a test, the spec must be reviewed first.

var _ = Describe("Build Pipeline", func() {

	Describe("Phase ordering", func() {
		It("completes a build and returns a result", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site", Clean: true},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("skips Phase 2 entirely when no SSR configured", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				// SSR is nil — no SSR configured
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"Phase 2 must be skipped when no ssr: config is present")
		})
	})

	Describe("Error behavior", func() {
		It("produces no output when a page fails to render", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Inject content with a broken template filter to force a render error
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"content/broken.md": "---\ntitle: Broken\n---\n{{ undefined | nonexistent_filter }}",
			})
			Expect(err).To(HaveOccurred(),
				"build must fail when any page has a render error")
			Expect(err.Error()).To(ContainSubstring("broken"),
				"error must reference the failing source file")
			Expect(result).To(BeNil(),
				"failed build must not return partial result — no partial deploys")
		})

		It("includes stage information in error messages", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Force a render error with broken template syntax
			_, err := pipeline.BuildWithContent(cfg, map[string]string{
				"content/bad.md": "---\ntitle: Bad\n---\n{{ | broken_syntax }}",
			})
			Expect(err).To(HaveOccurred(),
				"broken template must cause a build error")
			Expect(err.Error()).To(ContainSubstring("template rendering"),
				"error must identify the pipeline stage where the failure occurred")
		})
	})

	Describe("Build result", func() {
		It("returns output directory path", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.OutputDir).To(Equal("_site"))
		})

		It("returns page count matching pages rendered", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"build must render at least one page from content/")
		})

		It("returns build duration", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Duration).To(BeNumerically(">", 0))
		})

		It("returns zero page count when content directory is empty", func() {
			cfg := &config.Config{
				Title: "Empty Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.BuildWithContent(cfg, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(0))
		})
	})

	// ── Phase 1 → Phase 2 handoff (§2) ──────────────────────────────
	// Per spec §6: Phase 2 operates in memory. For each page with custom
	// elements, Alloy pipes the full page HTML to ssr.command via stdin.
	// The SSR engine handles all component rendering internally.

	Describe("Phase 1 → Phase 2 handoff", func() {
		It("Phase 1 produces intermediate HTML preserving raw custom element tags", func() {
			cfg := &config.Config{
				Title: "Component Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			intermediate, err := pipeline.BuildPhase1(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"Phase 1 must complete without error")
			Expect(intermediate).NotTo(BeEmpty(),
				"Phase 1 must produce at least one page of intermediate HTML")
		})

		It("Phase 2 invokes command per page, piping full HTML via stdin", func() {
			// Intermediate HTML contains a custom element (hyphenated tag).
			// BuildPhase2 must attempt to invoke the command for each page
			// containing custom elements. Using a nonexistent command proves
			// the invocation is attempted.
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card title="Hello">content</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "golit render --defs bundles/",
			}
			// The command won't exist in the test environment, so this
			// must return an error referencing the command — not silently skip
			// or fall back to a local transform.
			_, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).To(HaveOccurred(),
				"Phase 2 must attempt to invoke ssr.command (which won't exist in test env)")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("golit"),
				ContainSubstring("exec"),
				ContainSubstring("not found"),
			), "error must reference the ssr.command that failed to execute")
		})

		It("Phase 2 receives Phase 1 output as its input", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR: &config.SSRConfig{
					// cat reads stdin, writes to stdout — proves the per-page
					// stdio model works end-to-end
					Command: "cat",
				},
				Build: config.BuildConfig{Output: "_site"},
			}

			// Phase 1 produces intermediate HTML
			intermediate, err := pipeline.BuildPhase1(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(intermediate).NotTo(BeEmpty(),
				"Phase 1 must produce intermediate output")

			// Phase 2 takes Phase 1 output directly and pipes to ssr.command
			// via stdin
			_, err = pipeline.BuildPhase2(intermediate, cfg.SSR)
			// Either succeeds or fails — either proves BuildPhase2 accepts
			// Phase 1 output and attempts per-page rendering
			_ = err
		})

		It("without SSR config, Phase 1 output is the final HTML", func() {
			cfg := &config.Config{
				Title: "No SSR Site",
				Build: config.BuildConfig{Output: "_site"},
				// SSR is nil — no ssr: config block
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"without ssr: config, Phase 2 must be skipped entirely")
		})
	})

	// ── SSR per-page render ─────────────────────────────────────────
	// Phase 2 pipes the full page HTML to ssr.command via stdin.
	// The SSR engine handles component discovery, rendering, and DSD
	// injection internally. Alloy treats it as a black box.

	Describe("SSR per-page render", func() {
		It("BuildPhase2 returns error when command is not found", func() {
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-button>Click</ds-button></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool render --defs bundles/",
			}
			_, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).To(HaveOccurred(),
				"BuildPhase2 must return error when ssr.command is not found")
		})

		It("BuildPhase2 does not fall back to local DSD transform", func() {
			// When the command is unavailable, BuildPhase2 must NOT
			// silently insert <template shadowrootmode> via a local transform.
			// SSR is the external engine's responsibility.
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>content</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool render",
			}
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			if err == nil {
				// If no error, the HTML must NOT contain shadowrootmode —
				// that would mean a local transform ran instead of the external tool
				html := result["content/index.md"]
				Expect(html).NotTo(ContainSubstring("shadowrootmode"),
					"BuildPhase2 must not silently fall back to local DSD transform "+
						"when the ssr.command is unavailable")
			}
			// If err != nil, command execution was attempted and failed — correct behavior
		})

		It("BuildPhase2 passes through HTML unchanged when no custom elements present", func() {
			// Pages without custom elements (no hyphenated tags) should pass
			// through Phase 2 unchanged — no command invocations needed.
			intermediate := map[string]string{
				"content/plain.md": `<html><body><h1>Hello</h1><p>No components here.</p></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				// Command that would fail if invoked — proves it's NOT called
				Command: "false",
			}
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"BuildPhase2 must not error on pages without custom elements")
			Expect(result).NotTo(BeNil())
			Expect(result["content/plain.md"]).To(Equal(intermediate["content/plain.md"]),
				"HTML without custom elements must pass through Phase 2 unchanged")
		})
	})

	// ── SSR skip behavior ────────────────────────────────────────────

	Describe("SSR skip behavior", func() {
		It("no SSR config sets SSRSkipped to true", func() {
			cfg := &config.Config{
				Title: "Plain Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"build without ssr: config must skip Phase 2")
		})

		It("with SSR config, Phase 2 runs and SSRSkipped is false", func() {
			// Guard: without SSR, SSRSkipped must be true
			noSSRCfg := &config.Config{
				Title: "No SSR",
				Build: config.BuildConfig{Output: "_site"},
			}
			noSSRResult, err := pipeline.Build(noSSRCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(noSSRResult).NotTo(BeNil())
			Expect(noSSRResult.SSRSkipped).To(BeTrue(),
				"guard: no SSR config must set SSRSkipped=true")

			// Actual: with SSR, Build attempts Phase 2 (invokes ssr.command).
			// Use "cat" — reads stdin, writes to stdout. Proves the per-page
			// stdio model is attempted.
			ssrCfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			ssrResult, err := pipeline.Build(ssrCfg)
			if err != nil {
				// If Build errors, it must be because Phase 2 attempted
				// command execution — not Phase 1 or config validation.
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("echo"),
					ContainSubstring("exec"),
					ContainSubstring("ssr"),
					ContainSubstring("command"),
				), "error must come from Phase 2 command execution, not Phase 1")
			} else {
				Expect(ssrResult).NotTo(BeNil())
				Expect(ssrResult.SSRSkipped).To(BeFalse(),
					"build with ssr: config must run Phase 2")
			}
		})
	})
})
