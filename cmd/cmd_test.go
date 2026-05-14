package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/zeroedin/alloy/cmd"
)

var _ = Describe("CLI Commands", func() {

	// ── Command registration + execution ─────────────────────────────

	Describe("Command registration", func() {
		It("build command executes the build pipeline successfully", func() {
			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"build"})
			err := root.Execute()
			Expect(err).NotTo(HaveOccurred(),
				"alloy build must complete the pipeline without error")
		})

		It("dev command is registered and callable", func() {
			root := cmd.NewRootCommand()
			devCmd, _, err := root.Find([]string{"dev"})
			Expect(err).NotTo(HaveOccurred(),
				"dev command must be findable on root")
			Expect(devCmd).NotTo(BeNil())
			Expect(devCmd.Name()).To(Equal("dev"),
				"dev command must be registered on root — "+
					"alloy dev is the development server (Phase 1, in-memory, drafts visible)")
		})

		It("serve command is registered and callable", func() {
			root := cmd.NewRootCommand()
			serveCmd, _, err := root.Find([]string{"serve"})
			Expect(err).NotTo(HaveOccurred(),
				"serve command must be findable on root")
			Expect(serveCmd).NotTo(BeNil())
			Expect(serveCmd.Name()).To(Equal("serve"),
				"serve command must be registered on root — "+
					"alloy serve is the production server (same pipeline as build)")
		})

		It("init command executes successfully", func() {
			// Clean up CWD artifact from init (no directory arg defaults to ".")
			DeferCleanup(func() {
				os.Remove("alloy.config.yaml")
			})
			// Remove any leftover from a previous run so this test is idempotent
			os.Remove("alloy.config.yaml")

			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"init"})
			err := root.Execute()
			Expect(err).NotTo(HaveOccurred(),
				"alloy init must create default config without error")
		})

		It("version command executes and prints version", func() {
			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"version"})
			err := root.Execute()
			Expect(err).NotTo(HaveOccurred(),
				"alloy version must print version info without error")
		})
	})

	// ── Global flags (§9 Flags) ──────────────────────────────────────

	Describe("Global flags", func() {
		It("--config / -c defaults to alloy.config.yaml", func() {
			root := cmd.NewRootCommand()
			flag := root.PersistentFlags().Lookup("config")
			if flag == nil {
				Fail("--config flag must be registered on root command")
				return
			}
			Expect(flag.Shorthand).To(Equal("c"))
			Expect(flag.DefValue).To(Equal("alloy.config.yaml"))
		})

		It("--output / -o defaults to _site", func() {
			root := cmd.NewRootCommand()
			flag := root.PersistentFlags().Lookup("output")
			if flag == nil {
				Fail("--output flag must be registered on root command")
				return
			}
			Expect(flag.Shorthand).To(Equal("o"))
			Expect(flag.DefValue).To(Equal("_site"))
		})

		It("--verbose / -v is registered on root", func() {
			root := cmd.NewRootCommand()
			flag := root.PersistentFlags().Lookup("verbose")
			if flag == nil {
				Fail("--verbose flag must be registered on root command")
				return
			}
			Expect(flag.Shorthand).To(Equal("v"))
		})

		It("--quiet / -q is registered on root", func() {
			root := cmd.NewRootCommand()
			flag := root.PersistentFlags().Lookup("quiet")
			if flag == nil {
				Fail("--quiet flag must be registered on root command")
				return
			}
			Expect(flag.Shorthand).To(Equal("q"))
		})

		It("--root / -r defaults to empty string", func() {
			root := cmd.NewRootCommand()
			flag := root.PersistentFlags().Lookup("root")
			if flag == nil {
				Fail("--root flag must be registered on root command")
				return
			}
			Expect(flag.Shorthand).To(Equal("r"))
			Expect(flag.DefValue).To(Equal(""),
				"--root must default to empty (use config file directory)")
		})
	})

	// ── Dev command flags (§9 Flags, issue #256) ────────────────────

	Describe("Dev command flags", func() {
		var findDev = func(root *cobra.Command) *cobra.Command {
			for _, c := range root.Commands() {
				if c.Name() == "dev" {
					return c
				}
			}
			return nil
		}

		It("--port / -p defaults to 3000 on dev", func() {
			root := cmd.NewRootCommand()
			devCmd := findDev(root)
			Expect(devCmd).NotTo(BeNil(), "dev command must be registered")

			flag := devCmd.Flags().Lookup("port")
			if flag == nil {
				Fail("--port flag must be registered on dev command")
				return
			}
			Expect(flag.Shorthand).To(Equal("p"))
			Expect(flag.DefValue).To(Equal("3000"))
		})

		It("--no-drafts is registered on dev command", func() {
			root := cmd.NewRootCommand()
			devCmd := findDev(root)
			Expect(devCmd).NotTo(BeNil(), "dev command must be registered")

			flag := devCmd.Flags().Lookup("no-drafts")
			if flag == nil {
				Fail("--no-drafts flag must be registered on dev command — "+
					"dev mode shows drafts by default, --no-drafts hides them")
				return
			}
		})

		It("--refetch is registered on dev command", func() {
			root := cmd.NewRootCommand()
			devCmd := findDev(root)
			Expect(devCmd).NotTo(BeNil(), "dev command must be registered")

			flag := devCmd.Flags().Lookup("refetch")
			if flag == nil {
				Fail("--refetch flag must be registered on dev command")
				return
			}
		})
	})

	// ── Build command --profile flag (issue #389/461) ───────────────

	Describe("Build command --profile flag", func() {
		var findBuild = func(root *cobra.Command) *cobra.Command {
			for _, c := range root.Commands() {
				if c.Name() == "build" {
					return c
				}
			}
			return nil
		}

		It("--profile flag is registered on build command", func() {
			root := cmd.NewRootCommand()
			buildCmd := findBuild(root)
			Expect(buildCmd).NotTo(BeNil(), "build command must be registered")

			flag := buildCmd.Flags().Lookup("profile")
			Expect(flag).NotTo(BeNil(),
				"--profile flag must be registered on build command — "+
					"enables pprof profiling and per-stage timing (issue #389)")
			Expect(flag.DefValue).To(Equal("false"),
				"--profile must default to false")
		})

		It("--profile-dir flag is registered with default .alloy/profiles", func() {
			root := cmd.NewRootCommand()
			buildCmd := findBuild(root)
			Expect(buildCmd).NotTo(BeNil())

			flag := buildCmd.Flags().Lookup("profile-dir")
			Expect(flag).NotTo(BeNil(),
				"--profile-dir flag must be registered on build command — "+
					"specifies where cpu.prof and mem.prof are written")
			Expect(flag.DefValue).To(Equal(".alloy/profiles"),
				"--profile-dir must default to .alloy/profiles")
		})

		It("--profile with --root writes profiles to project root, not CWD", func() {
			projectDir, err := os.MkdirTemp("", "alloy-profile-root-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(projectDir)

			Expect(os.WriteFile(
				filepath.Join(projectDir, "alloy.config.yaml"),
				[]byte("title: \"Profile Root Test\"\nbaseURL: \"https://example.com\"\n"),
				0644,
			)).To(Succeed())

			contentDir := filepath.Join(projectDir, "content")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\n---\n# Home"),
				0644,
			)).To(Succeed())

			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"build", "--root", projectDir, "--profile"})
			err = root.Execute()
			Expect(err).NotTo(HaveOccurred())

			// Profiles must be in projectDir/.alloy/profiles/, not CWD/.alloy/profiles/
			profileDir := filepath.Join(projectDir, ".alloy", "profiles")
			_, statErr := os.Stat(filepath.Join(profileDir, "cpu.prof"))
			Expect(statErr).NotTo(HaveOccurred(),
				"cpu.prof must be written to <project-root>/.alloy/profiles/ — "+
					"not CWD. --profile-dir must resolve relative to cfg.ProjectRoot "+
					"when --root is used")
		})
	})

	// ── Serve command flags (§9 Flags, issue #256) ───────────────────

	Describe("Serve command flags", func() {
		var findServe = func(root *cobra.Command) *cobra.Command {
			for _, c := range root.Commands() {
				if c.Name() == "serve" {
					return c
				}
			}
			return nil
		}

		It("--port / -p defaults to 3000 on serve", func() {
			root := cmd.NewRootCommand()
			serveCmd := findServe(root)
			Expect(serveCmd).NotTo(BeNil(), "serve command must be registered")

			flag := serveCmd.Flags().Lookup("port")
			if flag == nil {
				Fail("--port flag must be registered on serve command")
				return
			}
			Expect(flag.Shorthand).To(Equal("p"))
			Expect(flag.DefValue).To(Equal("3000"))
		})

		It("--refetch is registered on serve command", func() {
			root := cmd.NewRootCommand()
			serveCmd := findServe(root)
			Expect(serveCmd).NotTo(BeNil(), "serve command must be registered")

			flag := serveCmd.Flags().Lookup("refetch")
			if flag == nil {
				Fail("--refetch flag must be registered on serve command")
				return
			}
		})

		It("--preview flag does NOT exist on serve command", func() {
			root := cmd.NewRootCommand()
			serveCmd := findServe(root)
			Expect(serveCmd).NotTo(BeNil(), "serve command must be registered")

			flag := serveCmd.Flags().Lookup("preview")
			Expect(flag).To(BeNil(),
				"--preview flag must not exist — alloy serve IS the production server, "+
					"the --preview flag was removed in #256")
		})

		It("--no-drafts flag does NOT exist on serve command", func() {
			root := cmd.NewRootCommand()
			serveCmd := findServe(root)
			Expect(serveCmd).NotTo(BeNil(), "serve command must be registered")

			flag := serveCmd.Flags().Lookup("no-drafts")
			Expect(flag).To(BeNil(),
				"--no-drafts must not exist on serve — "+
					"production server always excludes drafts, no flag needed")
		})
	})

	// ── --root config resolution (issue #380) ───────────────────────
	// When --root is set and --config is not explicitly provided,
	// the config file must be loaded from the root directory, not CWD.

	Describe("--root flag resolves config from root directory (issue #380)", func() {
		It("build with --root loads taxonomies from root config", func() {
			// The bug: --root sets ProjectRoot but config is loaded from CWD.
			// Config-dependent features (taxonomies, permalinks) are nil.
			// This test creates a project with taxonomies in its config and
			// verifies they're loaded when using --root without --config.
			projectDir, err := os.MkdirTemp("", "alloy-root-380-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(projectDir)

			// Config with taxonomies — will be nil if loaded from CWD
			Expect(os.WriteFile(
				filepath.Join(projectDir, "alloy.config.yaml"),
				[]byte("title: \"Root380\"\nbaseURL: \"https://example.com\"\ntaxonomies:\n  tags:\n    render: false\n"),
				0644,
			)).To(Succeed())

			// Content with tags — needs taxonomies config to be processed
			contentDir := filepath.Join(projectDir, "content")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(contentDir, "post.md"),
				[]byte("---\ntitle: Post\ntags: [\"go\"]\nlayout: default\n---\n# Post"),
				0644,
			)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Index\nlayout: default\n---\n{% for p in taxonomies.tags.go %}<span class=\"tagged\">{{ p.title }}</span>{% endfor %}"),
				0644,
			)).To(Succeed())

			layoutsDir := filepath.Join(projectDir, "layouts")
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644,
			)).To(Succeed())

			// Build with --root only (no --config)
			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"build", "--root", projectDir})
			err = root.Execute()
			Expect(err).NotTo(HaveOccurred())

			// Read the index output — if taxonomies loaded, tags.go is populated
			indexHTML, readErr := os.ReadFile(filepath.Join(projectDir, "_site", "index.html"))
			Expect(readErr).NotTo(HaveOccurred(),
				"index.html must be generated in _site/")
			Expect(string(indexHTML)).To(ContainSubstring("tagged"),
				"taxonomies.tags.go must be populated when using --root — "+
					"if empty, cfg.Taxonomies is nil because config was loaded from CWD "+
					"(empty defaults) instead of from the --root directory (issue #380)")
		})
	})

	// ── --root full config loading (issue #437) ─────────────────────
	// The -r flag must load ALL config fields from the project's config
	// file, not just some. Previously only taxonomies were tested (#380).
	// The plugins section and other fields are also lost.

	Describe("--root flag loads full config (issue #437)", func() {
		It("--root respects build.output from root config", func() {
			projectDir, err := os.MkdirTemp("", "alloy-root-437-compare-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(projectDir)

			// Config with multiple non-default fields
			configContent := `title: "Compare437"
baseURL: "https://example.com"
build:
  output: "dist"
plugins:
  timeout: 25000
taxonomies:
  tags:
    render: false
  categories:
    render: false
`
			Expect(os.WriteFile(
				filepath.Join(projectDir, "alloy.config.yaml"),
				[]byte(configContent),
				0644,
			)).To(Succeed())

			contentDir := filepath.Join(projectDir, "content")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\n---\n# Home"),
				0644,
			)).To(Succeed())

			// Build with --root
			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"build", "--root", projectDir})
			err = root.Execute()
			Expect(err).NotTo(HaveOccurred())

			// Config has build.output: "dist" — output must be in dist/, not _site/
			_, statErr := os.Stat(filepath.Join(projectDir, "dist", "index.html"))
			Expect(statErr).NotTo(HaveOccurred(),
				"output must be in 'dist/' (from config build.output) not '_site/' — "+
					"if _site/ exists instead, config was not loaded from --root. "+
					"The -r flag must load ALL config fields including build.output (issue #437)")
		})
	})

	// ── alloy init behavior ──────────────────────────────────────────

	Describe("alloy init", func() {
		It("creates alloy.config.yaml in the target directory", func() {
			tmpDir, err := os.MkdirTemp("", "alloy-init-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			err = cmd.RunInit(tmpDir)
			Expect(err).NotTo(HaveOccurred(),
				"RunInit must succeed when no config exists")

			configPath := filepath.Join(tmpDir, "alloy.config.yaml")
			_, err = os.Stat(configPath)
			Expect(err).NotTo(HaveOccurred(),
				"alloy.config.yaml must be created in target directory")
		})

		It("returns error mentioning 'already exists' when config is present", func() {
			tmpDir, err := os.MkdirTemp("", "alloy-init-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			// Pre-create config
			err = os.WriteFile(
				filepath.Join(tmpDir, "alloy.config.yaml"),
				[]byte("title: Existing Site"),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			err = cmd.RunInit(tmpDir)
			Expect(err).To(HaveOccurred(),
				"RunInit must fail when config already exists")
			Expect(err.Error()).To(ContainSubstring("already exists"),
				"error must explain that config already exists")
		})

		It("creates target directory if it does not exist", func() {
			tmpDir, err := os.MkdirTemp("", "alloy-init-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			nestedDir := filepath.Join(tmpDir, "new-project", "subdir")
			err = cmd.RunInit(nestedDir)
			Expect(err).NotTo(HaveOccurred(),
				"RunInit must create target directory if it does not exist")

			configPath := filepath.Join(nestedDir, "alloy.config.yaml")
			_, err = os.Stat(configPath)
			Expect(err).NotTo(HaveOccurred(),
				"alloy.config.yaml must exist in the created directory")
		})

		It("generated config includes baseURL so it passes validation", func() {
			tmpDir, err := os.MkdirTemp("", "alloy-init-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			err = cmd.RunInit(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			configPath := filepath.Join(tmpDir, "alloy.config.yaml")
			configBytes, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			configStr := string(configBytes)
			Expect(configStr).To(ContainSubstring("baseURL"),
				"generated config must include baseURL for config.Validate to pass")
		})

		It("init command returns error (not exit 0) when config already exists", func() {
			tmpDir, err := os.MkdirTemp("", "alloy-init-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			// Pre-create config
			err = os.WriteFile(
				filepath.Join(tmpDir, "alloy.config.yaml"),
				[]byte("title: Existing Site"),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"init", tmpDir})
			err = root.Execute()
			Expect(err).To(HaveOccurred(),
				"init command must return error when config exists — not swallow it and exit 0")
		})
	})

	// ── Dev/Serve watcher wiring (issue #371) ───────────────────────
	// The CLI split (#256) moved the watcher from serve.go to dev.go but
	// broke two things: dev calls Build() instead of BuildIncremental(),
	// and serve lost its watcher entirely. These tests verify the fix.
	//
	// These tests check for qualified call patterns (e.g., "pipeline.BuildIncremental")
	// rather than bare substrings to avoid false positives from comments.

	Describe("Dev watcher uses BuildIncremental (issue #371)", func() {
		It("dev command calls pipeline.BuildIncremental", func() {
			devSource, err := os.ReadFile("dev.go")
			Expect(err).NotTo(HaveOccurred(),
				"dev.go must exist in cmd/ package")
			Expect(string(devSource)).To(ContainSubstring("pipeline.BuildIncremental("),
				"dev.go must call pipeline.BuildIncremental() for watcher rebuilds — "+
					"not pipeline.Build(). Dev mode uses incremental rebuilds (PLAN.md §8)")
		})
	})

	Describe("Serve command has file watcher (issue #371)", func() {
		It("serve command imports fsnotify", func() {
			serveSource, err := os.ReadFile("serve.go")
			Expect(err).NotTo(HaveOccurred(),
				"serve.go must exist in cmd/ package")
			Expect(string(serveSource)).To(ContainSubstring("\"github.com/fsnotify/fsnotify\""),
				"serve.go must import fsnotify for file watching — "+
					"alloy serve is NOT a one-shot build (PLAN.md §8). "+
					"The CLI split (#256) removed the watcher; it must be restored")
		})

		It("serve command calls BroadcastReload", func() {
			serveSource, err := os.ReadFile("serve.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(serveSource)).To(ContainSubstring(".BroadcastReload("),
				"serve.go must call srv.BroadcastReload() after rebuilds — "+
					"file changes must trigger browser reload via WebSocket")
		})
	})

	// ── alloy version ────────────────────────────────────────────────

	Describe("alloy version", func() {
		It("Version variable is set to a non-empty build string", func() {
			Expect(cmd.Version).NotTo(BeEmpty(),
				"Version must be set (typically via ldflags at build time)")
		})
	})

	// ── CLI banner ──────────────────────────────────────────────────
	// The alloy logo displays as a Unicode block-character banner on
	// dev and serve startup. TTY mode adds two-tone ANSI coloring;
	// non-TTY mode uses plain Unicode only. Suppressed by --quiet.

	Describe("CLI banner", func() {
		It("PrintBanner writes the alloy logo to the writer", func() {
			var buf bytes.Buffer
			cmd.PrintBanner(&buf, false)
			output := buf.String()
			Expect(output).To(ContainSubstring("█▀▀█"),
				"banner must contain Unicode block characters forming the alloy logo")
			Expect(output).To(ContainSubstring("▀▄▄█"),
				"banner must contain the y character with descender")
		})

		It("PrintBanner includes ANSI color codes in TTY mode", func() {
			var buf bytes.Buffer
			cmd.PrintBanner(&buf, true)
			output := buf.String()
			Expect(output).To(ContainSubstring("\033["),
				"TTY mode banner must include ANSI escape sequences for two-tone coloring — "+
					"use bold/dim attributes (not hardcoded color values) so the banner "+
					"adapts to both light and dark terminal backgrounds")
		})

		It("PrintBanner omits ANSI codes in non-TTY mode", func() {
			var buf bytes.Buffer
			cmd.PrintBanner(&buf, false)
			output := buf.String()
			Expect(output).NotTo(ContainSubstring("\033["),
				"non-TTY banner must not include ANSI escape sequences — "+
					"raw block characters only for piped/redirected output")
		})

		It("dev command calls PrintBanner", func() {
			devSource, err := os.ReadFile("dev.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(devSource)).To(ContainSubstring("PrintBanner"),
				"dev.go must call PrintBanner — the alloy logo must display on startup")
		})

		It("serve command calls PrintBanner", func() {
			serveSource, err := os.ReadFile("serve.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(serveSource)).To(ContainSubstring("PrintBanner"),
				"serve.go must call PrintBanner — the alloy logo must display on startup")
		})
	})
})
