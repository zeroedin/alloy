package cmd_test

import (
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

	// ── alloy version ────────────────────────────────────────────────

	Describe("alloy version", func() {
		It("Version variable is set to a non-empty build string", func() {
			Expect(cmd.Version).NotTo(BeEmpty(),
				"Version must be set (typically via ldflags at build time)")
		})
	})
})
