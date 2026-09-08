package config_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
)

// ── Config-file write containment (issue #1254) ───────────────────────
//
// A path Alloy reads from may leave the project; a path Alloy writes to
// may not. Config-file values were never checked, so passthrough `to` and
// `build.output` could both place files outside the project entirely while
// the build reported success — verified on main with the built CLI:
//
//	passthrough to: "../../STOLEN"   -> wrote two levels above the root
//	build.output:   "../../ESCAPED"  -> wrote outside the project
//	passthrough to: "<absolute>"     -> silently became _site/<abs path>/…
//
// The plugin path (`onConfig`) already rejects all of these. The rule is
// about what Alloy may do to a filesystem, so it must hold for both sources.
//
// Spec: PLAN.md → "Write containment (issue #1254)".

var _ = Describe("Config-file write containment (issue #1254)", func() {

	baseCfg := func() *config.Config {
		cfg := &config.Config{
			Title:       "Containment Test",
			BaseURL:     "https://example.com",
			ProjectRoot: filepath.Join("/tmp", "project"),
			Build:       config.BuildConfig{Output: "_site"},
		}
		config.ApplyDefaults(cfg)
		return cfg
	}

	// ── Writes must stay inside their bound ───────────────────────────

	Context("passthrough to — writes into the output directory", func() {
		It("rejects a to that climbs above the output directory", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "../../STOLEN"},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"a passthrough to that escapes the output directory must fail "+
					"validation — today it writes outside the project and the "+
					"build reports success")
			Expect(err.Error()).To(ContainSubstring("passthrough[0].to"),
				"the error must name the offending field, matching the shape the "+
					"plugin path already uses")
			Expect(err.Error()).To(ContainSubstring("../../STOLEN"),
				"the error must quote the offending value")
		})

		It("rejects an absolute to instead of silently reinterpreting it", func() {
			// Today an absolute to is joined onto the output directory, so
			// "/srv/assets" quietly produces _site/srv/assets/… — a path the
			// author never asked for.
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "/srv/assets"},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"an absolute passthrough to must be rejected, not silently "+
					"reinterpreted as a path inside the output directory")
			Expect(err.Error()).To(ContainSubstring("passthrough[0].to"))
		})

		It("rejects a to that escapes only after the path collapses", func() {
			// Containment must be checked on the resolved path. A raw-string
			// check misses this, because the escape only appears once Join
			// and Clean have collapsed the traversal.
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "css/../../../outside"},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"containment must be evaluated after resolving and cleaning the "+
					"path, not on the raw string")
			Expect(err.Error()).To(ContainSubstring("passthrough[0].to"))
		})

		It("names the right index when a later mapping is the offender", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "css"},
				{From: "other", To: "../../STOLEN"},
			}
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("passthrough[1].to"),
				"the index must identify which mapping to fix")
		})
	})

	Context("build.output — writes into the project root", func() {
		It("rejects an output directory above the project root", func() {
			cfg := baseCfg()
			cfg.Build.Output = "../../ESCAPED_SITE"
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"build.output must resolve inside the project root — today it "+
					"writes the whole site outside the project and exits 0")
			Expect(err.Error()).To(ContainSubstring("build.output"))
			Expect(err.Error()).To(ContainSubstring("../../ESCAPED_SITE"))
		})

		It("rejects an absolute output directory", func() {
			cfg := baseCfg()
			cfg.Build.Output = "/var/www/html"
			err := config.Validate(cfg)
			Expect(err).To(HaveOccurred(),
				"an absolute build.output escapes the project root, and the "+
					"plugin path already rejects absolute paths")
			Expect(err.Error()).To(ContainSubstring("build.output"))
		})
	})

	// ── Reads keep their latitude, and legal writes keep working ──────

	Context("Reads are unconstrained", func() {
		It("allows a passthrough from outside the project", func() {
			// PLAN.md §1h deliberately supports absolute and ../relative from
			// values for cross-project asset sharing. The rule splits on
			// read-vs-write precisely so this keeps working.
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "../design-system/dist", To: "elements"},
			}
			Expect(config.Validate(cfg)).To(Succeed(),
				"a ../relative from reads from outside the project and must "+
					"remain legal — constraining it would break a documented feature")
		})

		It("allows an absolute passthrough from", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "/shared/assets", To: "vendor"},
			}
			Expect(config.Validate(cfg)).To(Succeed(),
				"an absolute from is supported for cross-project asset sharing")
		})
	})

	Context("Legal writes still validate", func() {
		It("accepts a nested to inside the output directory", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "assets/vendor/css"},
			}
			Expect(config.Validate(cfg)).To(Succeed(),
				"an ordinary nested to must keep working")
		})

		It("accepts a to that normalizes to a path inside the output directory", func() {
			// PLAN.md §2877 already accepts subdir/../dist, cleaning to dist.
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "subdir/../dist"},
			}
			Expect(config.Validate(cfg)).To(Succeed(),
				"traversal that stays inside the bound is legal — only escaping "+
					"the bound is an error")
		})

		It("accepts a to of \".\" meaning the output directory itself", func() {
			cfg := baseCfg()
			cfg.Passthrough = []config.PassthroughMapping{
				{From: "vendor", To: "."},
			}
			Expect(config.Validate(cfg)).To(Succeed(),
				"copying into the output directory root is legal for passthrough")
		})

		It("accepts a nested build.output inside the project", func() {
			cfg := baseCfg()
			cfg.Build.Output = "dist/site"
			Expect(config.Validate(cfg)).To(Succeed(),
				"any output directory under the project root is legal")
		})
	})
})
