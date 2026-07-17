package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/zeroedin/alloy/cmd"
	"github.com/zeroedin/alloy/internal/config"
)

// runInitCmd executes alloy init via cobra and returns any error.
func runInitCmd(dir string, flags ...string) error {
	root := cmd.NewRootCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(append([]string{"init", dir}, flags...))
	return root.Execute()
}

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
			tmpDir, err := os.MkdirTemp("", "alloy-init-smoke-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"init", tmpDir})
			err = root.Execute()
			Expect(err).NotTo(HaveOccurred(),
				"alloy init must scaffold a project without error")
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

	// ── alloy init scaffolding ───────────────────────────────────────

	Describe("alloy init", func() {

		// ── Fresh project (no config exists) ─────────────────────────

		Context("fresh project (no config exists)", func() {
			It("creates alloy.config.yaml in the target directory", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				_, err = os.Stat(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred(),
					"alloy.config.yaml must be created in target directory")
			})

			It("creates all six project directories", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				for _, name := range []string{"content", "layouts", "assets", "static", "data", "plugins"} {
					info, err := os.Stat(filepath.Join(tmpDir, name))
					Expect(err).NotTo(HaveOccurred(),
						"%s/ directory must be created by init", name)
					Expect(info.IsDir()).To(BeTrue(),
						"%s must be a directory", name)
				}
			})

			It("creates layouts/default.liquid with HTML5 shell", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				content, err := os.ReadFile(filepath.Join(tmpDir, "layouts", "default.liquid"))
				Expect(err).NotTo(HaveOccurred(),
					"layouts/default.liquid must be created by init")
				s := string(content)
				Expect(s).To(ContainSubstring("<!DOCTYPE html>"),
					"default layout must be a valid HTML5 document")
				Expect(s).To(ContainSubstring("{{ page.title }}"),
					"default layout must reference {{ page.title }} in the <head>")
				Expect(s).To(ContainSubstring("{{ content }}"),
					"default layout must inject page content via {{ content }}")
				Expect(s).To(ContainSubstring("/style.css"),
					"default layout must link to /style.css")
			})

			It("creates content/index.md with title and layout frontmatter", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				content, err := os.ReadFile(filepath.Join(tmpDir, "content", "index.md"))
				Expect(err).NotTo(HaveOccurred(),
					"content/index.md must be created by init")
				s := string(content)
				Expect(s).To(HavePrefix("---"),
					"index.md must start with YAML frontmatter delimiters")
				Expect(s).To(ContainSubstring("title:"),
					"index.md frontmatter must include a title")
				Expect(s).To(ContainSubstring("layout: default"),
					"index.md must reference layout: default to use the starter layout")
			})

			It("creates static/style.css with content", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				content, err := os.ReadFile(filepath.Join(tmpDir, "static", "style.css"))
				Expect(err).NotTo(HaveOccurred(),
					"static/style.css must be created by init")
				Expect(content).NotTo(BeEmpty(),
					"style.css must contain CSS — minimal reset and readable styling "+
						"so the starter page is not completely unstyled")
			})

			It("generated config passes config.Validate()", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				cfg, err := config.LoadWithDefaults(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred(),
					"generated config must be parseable")
				Expect(config.Validate(cfg)).To(Succeed(),
					"generated config must pass validation — "+
						"requires at minimum title and a valid baseURL")
			})

			It("does not write structure: block when all directories are defaults", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir)).To(Succeed())

				configBytes, err := os.ReadFile(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(configBytes)).NotTo(ContainSubstring("structure:"),
					"config with all-default directory names must not include a structure: block — "+
						"the pipeline defaults handle it")
			})

			It("creates target directory if it does not exist", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				nestedDir := filepath.Join(tmpDir, "new-project", "subdir")
				Expect(runInitCmd(nestedDir)).To(Succeed(),
					"init must create the target directory tree if it does not exist")

				_, err = os.Stat(filepath.Join(nestedDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred(),
					"alloy.config.yaml must exist in the created directory")
			})
		})

		// ── Custom structure flags ───────────────────────────────────

		Context("custom structure flags", func() {
			It("--content=pages creates pages/ and places index.md there", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--content=pages")).To(Succeed())

				_, err = os.Stat(filepath.Join(tmpDir, "pages", "index.md"))
				Expect(err).NotTo(HaveOccurred(),
					"index.md must be placed in the --content directory")
				_, err = os.Stat(filepath.Join(tmpDir, "content"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"default content/ must not be created when --content overrides it")
			})

			It("--layouts=templates creates templates/ and places default.liquid there", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--layouts=templates")).To(Succeed())

				_, err = os.Stat(filepath.Join(tmpDir, "templates", "default.liquid"))
				Expect(err).NotTo(HaveOccurred(),
					"default.liquid must be placed in the --layouts directory")
				_, err = os.Stat(filepath.Join(tmpDir, "layouts"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"default layouts/ must not be created when --layouts overrides it")
			})

			It("--static=public creates public/ and places style.css there", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--static=public")).To(Succeed())

				_, err = os.Stat(filepath.Join(tmpDir, "public", "style.css"))
				Expect(err).NotTo(HaveOccurred(),
					"style.css must be placed in the --static directory")
				_, err = os.Stat(filepath.Join(tmpDir, "static"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"default static/ must not be created when --static overrides it")
			})

			It("--assets=resources creates resources/ instead of assets/", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--assets=resources")).To(Succeed())

				info, err := os.Stat(filepath.Join(tmpDir, "resources"))
				Expect(err).NotTo(HaveOccurred(),
					"resources/ directory must be created by --assets flag")
				Expect(info.IsDir()).To(BeTrue())
				_, err = os.Stat(filepath.Join(tmpDir, "assets"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"default assets/ must not be created when --assets overrides it")
			})

			It("--data=datasets creates datasets/ instead of data/", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--data=datasets")).To(Succeed())

				info, err := os.Stat(filepath.Join(tmpDir, "datasets"))
				Expect(err).NotTo(HaveOccurred(),
					"datasets/ directory must be created by --data flag")
				Expect(info.IsDir()).To(BeTrue())
				_, err = os.Stat(filepath.Join(tmpDir, "data"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"default data/ must not be created when --data overrides it")
			})

			It("all five flags rename their directories correctly", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir,
					"--content=pages",
					"--layouts=templates",
					"--assets=resources",
					"--static=public",
					"--data=datasets",
				)).To(Succeed())

				for _, name := range []string{"pages", "templates", "resources", "public", "datasets", "plugins"} {
					info, err := os.Stat(filepath.Join(tmpDir, name))
					Expect(err).NotTo(HaveOccurred(),
						"%s/ directory must be created", name)
					Expect(info.IsDir()).To(BeTrue())
				}
				for _, name := range []string{"content", "layouts", "assets", "static", "data"} {
					_, err := os.Stat(filepath.Join(tmpDir, name))
					Expect(os.IsNotExist(err)).To(BeTrue(),
						"default %s/ must not be created when overridden by flag", name)
				}
			})

			It("--plugins=tools/plugins creates tools/plugins/ instead of plugins/ (issue #802)", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--plugins=tools/plugins")).To(Succeed())

				info, err := os.Stat(filepath.Join(tmpDir, "tools", "plugins"))
				Expect(err).NotTo(HaveOccurred(),
					"tools/plugins/ directory must be created by --plugins flag (issue #802)")
				Expect(info.IsDir()).To(BeTrue())
				_, err = os.Stat(filepath.Join(tmpDir, "plugins"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"default plugins/ must not be created when --plugins overrides it (issue #802)")
			})

			It("--plugins=tools/plugins writes plugins to config structure block (issue #802)", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--plugins=tools/plugins")).To(Succeed())

				cfg, err := config.Load(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Structure.Plugins).To(Equal("tools/plugins"),
					"structure.plugins must reflect the --plugins flag value (issue #802)")
			})

			It("custom flags write structure: block to config", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--content=pages", "--layouts=templates")).To(Succeed())

				cfg, err := config.Load(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Structure.Content).To(Equal("pages"),
					"structure.content must reflect the --content flag value")
				Expect(cfg.Structure.Layouts).To(Equal("templates"),
					"structure.layouts must reflect the --layouts flag value")
			})

			It("only non-default values appear in structure: block", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--content=pages")).To(Succeed())

				cfg, err := config.Load(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Structure.Content).To(Equal("pages"),
					"custom content path must be written to config")
				Expect(cfg.Structure.Layouts).To(BeEmpty(),
					"default layouts value must not be written to config — "+
						"only non-default structure values should appear")
				Expect(cfg.Structure.Assets).To(BeEmpty(),
					"default assets value must not be written to config")
				Expect(cfg.Structure.Static).To(BeEmpty(),
					"default static value must not be written to config")
				Expect(cfg.Structure.Data).To(BeEmpty(),
					"default data value must not be written to config")
			})

			It("generated config with custom flags passes config.Validate()", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(runInitCmd(tmpDir, "--content=pages", "--static=public")).To(Succeed())

				cfg, err := config.LoadWithDefaults(filepath.Join(tmpDir, "alloy.config.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(config.Validate(cfg)).To(Succeed(),
					"generated config with custom structure must still pass validation")
			})
		})

		// ── Existing project (no-op) ─────────────────────────────────

		Context("existing project (config already exists)", func() {
			DescribeTable("returns nil for all config extensions",
				func(ext string, content string) {
					tmpDir, err := os.MkdirTemp("", "alloy-init-*")
					Expect(err).NotTo(HaveOccurred())
					defer os.RemoveAll(tmpDir)

					configFile := filepath.Join(tmpDir, "alloy.config"+ext)
					Expect(os.WriteFile(configFile, []byte(content), 0644)).To(Succeed())

					Expect(runInitCmd(tmpDir)).To(Succeed(),
						"init must be a no-op (not an error) when a config file already exists — "+
							"use config.DetectConfigFile to check all four extensions")
				},
				Entry(".yaml", ".yaml", "title: Existing\n"),
				Entry(".yml", ".yml", "title: Existing\n"),
				Entry(".toml", ".toml", "title = \"Existing\"\n"),
				Entry(".json", ".json", "{\"title\": \"Existing\"}\n"),
			)

			It("does not create directories or starter files when config exists", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(os.WriteFile(
					filepath.Join(tmpDir, "alloy.config.yaml"),
					[]byte("title: Existing\nbaseURL: \"http://localhost:3000\"\n"),
					0644,
				)).To(Succeed())

				Expect(runInitCmd(tmpDir)).To(Succeed())

				for _, name := range []string{"content", "layouts", "assets", "static", "data", "plugins"} {
					_, err := os.Stat(filepath.Join(tmpDir, name))
					Expect(os.IsNotExist(err)).To(BeTrue(),
						"%s/ must not be created when a config file already exists", name)
				}
				_, err = os.Stat(filepath.Join(tmpDir, "layouts", "default.liquid"))
				Expect(os.IsNotExist(err)).To(BeTrue(),
					"starter files must not be created when a config file already exists")
			})

			It("preserves existing config file content unchanged", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				originalContent := "title: My Custom Site\nbaseURL: \"https://example.com\"\nlanguage: \"en\"\n"
				configPath := filepath.Join(tmpDir, "alloy.config.yaml")
				Expect(os.WriteFile(configPath, []byte(originalContent), 0644)).To(Succeed())

				Expect(runInitCmd(tmpDir)).To(Succeed())

				afterContent, err := os.ReadFile(configPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(afterContent)).To(Equal(originalContent),
					"init must not modify an existing config file — "+
						"the no-op path must leave the file byte-for-byte identical")
			})

			It("prints message containing 'already exists'", func() {
				tmpDir, err := os.MkdirTemp("", "alloy-init-*")
				Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tmpDir)

				Expect(os.WriteFile(
					filepath.Join(tmpDir, "alloy.config.yaml"),
					[]byte("title: Existing\nbaseURL: \"http://localhost:3000\"\n"),
					0644,
				)).To(Succeed())

				var buf bytes.Buffer
				root := cmd.NewRootCommand()
				root.SilenceErrors = true
				root.SilenceUsage = true
				root.SetOut(&buf)
				root.SetArgs([]string{"init", tmpDir})
				Expect(root.Execute()).To(Succeed())

				Expect(buf.String()).To(ContainSubstring("already exists"),
					"init must print a message like 'alloy project already exists in <dir>' — "+
						"the user needs to know init detected an existing project and did nothing")
			})
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

		// ── alloy version --check (issue #1071) ─────────────────────
		// The --check flag queries the GitHub Releases API for the latest
		// version and prints whether an update is available. It must be
		// registered on the version subcommand, not on root.

		It("version command has --check flag registered", func() {
			root := cmd.NewRootCommand()
			versionCmd, _, err := root.Find([]string{"version"})
			Expect(err).NotTo(HaveOccurred(),
				"version command must be registered on root")
			checkFlag := versionCmd.Flags().Lookup("check")
			Expect(checkFlag).NotTo(BeNil(),
				"--check flag must be registered on the version command — "+
					"this flag triggers an explicit update check against the "+
					"GitHub Releases API (issue #1071)")
			Expect(checkFlag.DefValue).To(Equal("false"),
				"--check must default to false — without it, alloy version "+
					"prints the version and exits (existing behavior)")
		})

		It("version command without --check does not print update info", func() {
			root := cmd.NewRootCommand()
			root.SilenceErrors = true
			root.SilenceUsage = true
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetArgs([]string{"version"})

			err := root.Execute()
			Expect(err).NotTo(HaveOccurred(),
				"alloy version without --check must succeed")
			output := buf.String()
			Expect(output).To(ContainSubstring("alloy"),
				"must print the version string")
			Expect(output).NotTo(ContainSubstring("Update available"),
				"without --check, must not print update information — "+
					"the existing behavior is a simple version print")
			Expect(output).NotTo(ContainSubstring("up to date"),
				"without --check, must not print update status")
		})
	})

	// ── alloy build must not reference update package (issue #1071) ──
	// The passive update check runs only in alloy dev and alloy serve.
	// alloy build must never check for updates — it is the CI command.
	// Verify at the source level that build.go does not import the
	// update package, preventing accidental wiring.

	Describe("alloy build does not reference update package (issue #1071)", func() {
		It("build.go does not import internal/update", func() {
			buildSource, err := os.ReadFile("build.go")
			Expect(err).NotTo(HaveOccurred(),
				"build.go must exist in the cmd/ package")
			Expect(string(buildSource)).NotTo(ContainSubstring("internal/update"),
				"build.go must NOT import the update package — alloy build "+
					"is the CI command and must never make outbound network "+
					"requests for version checking (issue #1071). The passive "+
					"update check is only wired into dev.go and serve.go.")
			Expect(string(buildSource)).NotTo(ContainSubstring("update.Check"),
				"build.go must NOT call any update.Check* function — "+
					"even without an import, ensure no transitive reference")
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
			Expect(string(devSource)).To(ContainSubstring("PrintBanner("),
				"dev.go must call PrintBanner() — the alloy logo must display on startup")
		})

		It("serve command calls PrintBanner", func() {
			serveSource, err := os.ReadFile("serve.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(serveSource)).To(ContainSubstring("PrintBanner("),
				"serve.go must call PrintBanner() — the alloy logo must display on startup")
		})
	})
})
