package plugin_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("Hooks", func() {

	// ── Hook registration ──────────────────────────────────────────────

	Describe("Hook registration", func() {
		It("registers a hook for onContentLoaded", func() {
			registry := plugin.NewHookRegistry()
			fn := func(_ context.Context, payload interface{}) (interface{}, error) {
				return payload, nil
			}
			registry.Register(plugin.OnContentLoaded, fn)
			result, err := registry.Run(plugin.OnContentLoaded, "test")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("test"))
		})

		It("registers multiple hooks for the same event", func() {
			registry := plugin.NewHookRegistry()
			fn1 := func(_ context.Context, payload interface{}) (interface{}, error) {
				return payload, nil
			}
			fn2 := func(_ context.Context, payload interface{}) (interface{}, error) {
				return payload, nil
			}
			registry.Register(plugin.OnContentLoaded, fn1)
			registry.Register(plugin.OnContentLoaded, fn2)
			result, err := registry.Run(plugin.OnContentLoaded, "data")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("data"))
		})
	})

	// ── Hook execution ─────────────────────────────────────────────────

	Describe("Hook execution", func() {
		It("executes hooks in registration order", func() {
			registry := plugin.NewHookRegistry()
			var order []int
			registry.Register(plugin.OnBuildComplete, func(_ context.Context, payload interface{}) (interface{}, error) {
				order = append(order, 1)
				return payload, nil
			})
			registry.Register(plugin.OnBuildComplete, func(_ context.Context, payload interface{}) (interface{}, error) {
				order = append(order, 2)
				return payload, nil
			})
			_, err := registry.Run(plugin.OnBuildComplete, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]int{1, 2}))
		})

		It("passes payload to hook function", func() {
			registry := plugin.NewHookRegistry()
			var received interface{}
			registry.Register(plugin.OnConfig, func(_ context.Context, payload interface{}) (interface{}, error) {
				received = payload
				return payload, nil
			})
			_, err := registry.Run(plugin.OnConfig, "my-payload")
			Expect(err).NotTo(HaveOccurred())
			Expect(received).To(Equal("my-payload"))
		})

		It("chains output: each hook receives previous hook's output", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnContentLoaded, func(_ context.Context, payload interface{}) (interface{}, error) {
				return payload.(string) + "-first", nil
			})
			registry.Register(plugin.OnContentLoaded, func(_ context.Context, payload interface{}) (interface{}, error) {
				return payload.(string) + "-second", nil
			})
			result, err := registry.Run(plugin.OnContentLoaded, "start")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("start-first-second"))
		})

		It("returns modified payload from hook chain", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnConfig, func(_ context.Context, payload interface{}) (interface{}, error) {
				m := payload.(map[string]string)
				m["added"] = "value"
				return m, nil
			})
			input := map[string]string{"original": "data"}
			result, err := registry.Run(plugin.OnConfig, input)
			Expect(err).NotTo(HaveOccurred())
			resultMap, ok := result.(map[string]string)
			Expect(ok).To(BeTrue())
			Expect(resultMap).To(HaveKeyWithValue("added", "value"))
			Expect(resultMap).To(HaveKeyWithValue("original", "data"))
		})
	})

	// ── Hook timeout ──────────────────────────────────────────────────

	Describe("Hook timeout", func() {
		It("default timeout is 5000ms", func() {
			registry := plugin.NewHookRegistry()
			Expect(registry.Timeout()).To(Equal(5000))
		})

		It("SetTimeout overrides the default", func() {
			registry := plugin.NewHookRegistry()
			registry.SetTimeout(10000)
			Expect(registry.Timeout()).To(Equal(10000))
		})

		It("hook completing within timeout returns its result", func() {
			registry := plugin.NewHookRegistry()
			registry.SetTimeout(5000)
			registry.Register(plugin.OnConfig, func(_ context.Context, payload interface{}) (interface{}, error) {
				// Fast hook — well within timeout
				return payload.(string) + "-modified", nil
			})

			result, err := registry.RunWithTimeout(plugin.OnConfig, "data")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("data-modified"))
			Expect(registry.Warnings()).To(BeEmpty())
		})

		It("hook exceeding timeout discards its modifications and keeps pre-hook payload", func() {
			registry := plugin.NewHookRegistry()
			registry.SetTimeout(50) // 50ms timeout for test speed

			registry.Register(plugin.OnContentLoaded, func(_ context.Context, payload interface{}) (interface{}, error) {
				// Simulate a slow hook that exceeds the timeout
				time.Sleep(200 * time.Millisecond)
				return "should-be-discarded", nil
			})

			result, err := registry.RunWithTimeout(plugin.OnContentLoaded, "original-payload")
			Expect(err).NotTo(HaveOccurred(), "timeout should not produce an error, build continues")
			Expect(result).To(Equal("original-payload"),
				"pre-hook payload must be preserved when hook times out")
		})

		It("hook exceeding timeout produces a warning naming the event", func() {
			registry := plugin.NewHookRegistry()
			registry.SetTimeout(50) // 50ms timeout for test speed

			registry.Register(plugin.OnContentTransformed, func(_ context.Context, payload interface{}) (interface{}, error) {
				time.Sleep(200 * time.Millisecond)
				return payload, nil
			})

			_, err := registry.RunWithTimeout(plugin.OnContentTransformed, "data")
			Expect(err).NotTo(HaveOccurred())

			warnings := registry.Warnings()
			Expect(warnings).NotTo(BeEmpty(), "timeout should produce a warning")
			Expect(warnings[0]).To(SatisfyAll(
				ContainSubstring("timeout"),
				ContainSubstring("onContentTransformed"),
			), "warning should name the event and mention timeout")
		})
	})

	// ── Lifecycle event constants ──────────────────────────────────────

	Describe("Lifecycle event constants", func() {
		It("OnConfig constant equals onConfig and hooks execute for it", func() {
			Expect(string(plugin.OnConfig)).To(Equal("onConfig"))

			// Constant alone is trivially true; verify hooks actually fire for this event
			registry := plugin.NewHookRegistry()
			called := false
			registry.Register(plugin.OnConfig, func(_ context.Context, payload interface{}) (interface{}, error) {
				called = true
				return payload, nil
			})
			result, err := registry.Run(plugin.OnConfig, "config-data")
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue(), "hook must be invoked for OnConfig event")
			Expect(result).To(Equal("config-data"))
		})

		It("OnContentLoaded constant equals onContentLoaded and hooks execute for it", func() {
			Expect(string(plugin.OnContentLoaded)).To(Equal("onContentLoaded"))

			registry := plugin.NewHookRegistry()
			called := false
			registry.Register(plugin.OnContentLoaded, func(_ context.Context, payload interface{}) (interface{}, error) {
				called = true
				return payload, nil
			})
			result, err := registry.Run(plugin.OnContentLoaded, "content-data")
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue(), "hook must be invoked for OnContentLoaded event")
			Expect(result).To(Equal("content-data"))
		})

		It("OnBuildComplete constant equals onBuildComplete and hooks execute for it", func() {
			Expect(string(plugin.OnBuildComplete)).To(Equal("onBuildComplete"))

			registry := plugin.NewHookRegistry()
			called := false
			registry.Register(plugin.OnBuildComplete, func(_ context.Context, payload interface{}) (interface{}, error) {
				called = true
				return payload, nil
			})
			result, err := registry.Run(plugin.OnBuildComplete, "build-data")
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue(), "hook must be invoked for OnBuildComplete event")
			Expect(result).To(Equal("build-data"))
		})
	})

	// ── Remaining lifecycle hook events ────────────────────────────────

	Describe("Remaining lifecycle hooks", func() {
		It("onBeforeValidation hook can append to output path map", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnBeforeValidation, func(_ context.Context, payload interface{}) (interface{}, error) {
				paths := payload.(map[string]string)
				paths["/api/data.json"] = "plugin:my-api"
				return paths, nil
			})
			input := map[string]string{"/index.html": "content/index.md"}
			result, err := registry.Run(plugin.OnBeforeValidation, input)
			Expect(err).NotTo(HaveOccurred())
			resultMap := result.(map[string]string)
			Expect(resultMap).To(HaveKey("/api/data.json"),
				"onBeforeValidation must allow mutation of output paths")
		})

		It("onAfterValidation hook receives read-only validation results", func() {
			registry := plugin.NewHookRegistry()
			var received interface{}
			registry.Register(plugin.OnAfterValidation, func(_ context.Context, payload interface{}) (interface{}, error) {
				received = payload
				return payload, nil
			})
			validationResult := map[string]interface{}{"conflicts": 0, "valid": true}
			_, err := registry.Run(plugin.OnAfterValidation, validationResult)
			Expect(err).NotTo(HaveOccurred())
			Expect(received).To(Equal(validationResult))
		})

		It("onDataFetched hook can modify fetched data", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnDataFetched, func(_ context.Context, payload interface{}) (interface{}, error) {
				data := payload.(map[string]interface{})
				data["injected"] = true
				return data, nil
			})
			input := map[string]interface{}{"source": "api"}
			result, err := registry.Run(plugin.OnDataFetched, input)
			Expect(err).NotTo(HaveOccurred())
			resultMap := result.(map[string]interface{})
			Expect(resultMap).To(HaveKey("injected"))
		})

		It("onDataCascadeReady hook can inspect cascade", func() {
			registry := plugin.NewHookRegistry()
			var inspected bool
			registry.Register(plugin.OnDataCascadeReady, func(_ context.Context, payload interface{}) (interface{}, error) {
				inspected = true
				return payload, nil
			})
			_, err := registry.Run(plugin.OnDataCascadeReady, "cascade-data")
			Expect(err).NotTo(HaveOccurred())
			Expect(inspected).To(BeTrue(),
				"onDataCascadeReady hook must execute")
		})

		It("onContentTransformed hook can modify rendered HTML", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnContentTransformed, func(_ context.Context, payload interface{}) (interface{}, error) {
				html := payload.(string)
				return html + "<!-- injected by plugin -->", nil
			})
			result, err := registry.Run(plugin.OnContentTransformed, "<p>Content</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>Content</p><!-- injected by plugin -->"))
		})

		It("onPageRendered hook post-processes page HTML", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnPageRendered, func(_ context.Context, payload interface{}) (interface{}, error) {
				return payload.(string) + "\n<!-- minified -->", nil
			})
			result, err := registry.Run(plugin.OnPageRendered, "<html></html>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("<!-- minified -->"))
		})

		It("onAssetProcess hook receives asset file path", func() {
			registry := plugin.NewHookRegistry()
			var receivedPath interface{}
			registry.Register(plugin.OnAssetProcess, func(_ context.Context, payload interface{}) (interface{}, error) {
				receivedPath = payload
				return payload, nil
			})
			_, err := registry.Run(plugin.OnAssetProcess, "assets/style.css")
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedPath).To(Equal("assets/style.css"))
		})

		It("onDevServerStart hook fires on server start", func() {
			registry := plugin.NewHookRegistry()
			var fired bool
			registry.Register(plugin.OnDevServerStart, func(_ context.Context, payload interface{}) (interface{}, error) {
				fired = true
				return payload, nil
			})
			_, err := registry.Run(plugin.OnDevServerStart, map[string]interface{}{"port": 3000})
			Expect(err).NotTo(HaveOccurred())
			Expect(fired).To(BeTrue())
		})

		It("onFileChanged hook receives changed file path", func() {
			registry := plugin.NewHookRegistry()
			var changedFile interface{}
			registry.Register(plugin.OnFileChanged, func(_ context.Context, payload interface{}) (interface{}, error) {
				changedFile = payload
				return payload, nil
			})
			_, err := registry.Run(plugin.OnFileChanged, "content/blog/post.md")
			Expect(err).NotTo(HaveOccurred())
			Expect(changedFile).To(Equal("content/blog/post.md"))
		})
	})

	// ── Cascade data shared pointer (§3) ─────────────────────────────

	Describe("Cascade data shared pointer", func() {
		It("hooks receiving cascade data get the shared pointer and mutations apply globally", func() {
			registry := plugin.NewHookRegistry()

			// Simulate cascade data as a shared pointer
			cascadeData := map[string]interface{}{
				"site": map[string]interface{}{
					"title": "Original Title",
				},
			}

			// Hook mutates the cascade data
			registry.Register(plugin.OnDataCascadeReady, func(_ context.Context, payload interface{}) (interface{}, error) {
				data := payload.(map[string]interface{})
				site := data["site"].(map[string]interface{})
				site["injected"] = "plugin-value"
				return data, nil
			})

			result, err := registry.Run(plugin.OnDataCascadeReady, cascadeData)
			Expect(err).NotTo(HaveOccurred())

			// The returned result must reflect the mutation
			resultMap := result.(map[string]interface{})
			resultSite := resultMap["site"].(map[string]interface{})
			Expect(resultSite).To(HaveKeyWithValue("injected", "plugin-value"),
				"hook must be able to mutate cascade data via shared pointer")

			// The ORIGINAL cascadeData must also reflect the mutation (shared pointer)
			origSite := cascadeData["site"].(map[string]interface{})
			Expect(origSite).To(HaveKeyWithValue("injected", "plugin-value"),
				"mutations must apply globally — hooks receive shared pointer, not a copy")
		})
	})

	// ── Batch hook dispatch ───────────────────────────────────────────

	Describe("Batch hook dispatch", func() {
		It("RegisterBatchWithPriority respects priority ordering", func() {
			registry := plugin.NewHookRegistry()
			var order []string

			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			batchA := func(_ context.Context, ps []interface{}, _ plugin.BatchProgressFunc) ([]interface{}, error) {
				order = append(order, "A")
				return ps, nil
			}
			batchB := func(_ context.Context, ps []interface{}, _ plugin.BatchProgressFunc) ([]interface{}, error) {
				order = append(order, "B")
				return ps, nil
			}

			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, batchB, 100)
			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, batchA, 10)

			_, err := registry.RunBatchWithTimeout(plugin.OnPageRendered, []interface{}{"x"})
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"A", "B"}),
				"lower priority must run first")
		})

		It("RunBatchWithTimeout returns error on result length mismatch", func() {
			registry := plugin.NewHookRegistry()
			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			batchFn := func(_ context.Context, ps []interface{}, _ plugin.BatchProgressFunc) ([]interface{}, error) {
				return ps[:1], nil // return fewer results
			}
			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, batchFn, 50)

			_, err := registry.RunBatchWithTimeout(plugin.OnPageRendered, []interface{}{"a", "b", "c"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("returned 1 results for 3 inputs"))
		})

		It("RunBatchWithTimeout per-item fallback enforces timeout", func() {
			registry := plugin.NewHookRegistry()
			registry.SetTimeout(50)

			slowFn := func(ctx context.Context, p interface{}) (interface{}, error) {
				select {
				case <-time.After(500 * time.Millisecond):
					return "modified", nil
				case <-ctx.Done():
					return p, ctx.Err()
				}
			}
			registry.Register(plugin.OnPageRendered, slowFn)

			results, err := registry.RunBatchWithTimeout(plugin.OnPageRendered, []interface{}{"a", "b"})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(Equal([]interface{}{"a", "b"}),
				"timed-out per-item hooks must preserve original values")
			Expect(registry.Warnings()).NotTo(BeEmpty(),
				"timeout must produce a warning")
		})

		It("RunBatchWithTimeout reverts to pre-hook state on batch timeout", func() {
			registry := plugin.NewHookRegistry()
			// timeout=10ms × 2 items = 20ms total budget
			// slowBatch sleeps 100ms, exceeds the 20ms budget
			registry.SetTimeout(10)

			fastBatch := func(_ context.Context, ps []interface{}, _ plugin.BatchProgressFunc) ([]interface{}, error) {
				out := make([]interface{}, len(ps))
				for i, p := range ps {
					out[i] = p.(string) + "-fast"
				}
				return out, nil
			}
			slowBatch := func(_ context.Context, ps []interface{}, _ plugin.BatchProgressFunc) ([]interface{}, error) {
				time.Sleep(100 * time.Millisecond)
				out := make([]interface{}, len(ps))
				for i, p := range ps {
					out[i] = p.(string) + "-slow"
				}
				return out, nil
			}
			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}

			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, fastBatch, 10)
			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, slowBatch, 20)

			results, err := registry.RunBatchWithTimeout(plugin.OnPageRendered, []interface{}{"a", "b"})
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(Equal([]interface{}{"a-fast", "b-fast"}),
				"timeout must revert to pre-hook state (after fast hook), not original input")
		})
	})

	// ── Batch progress callback (issue #686) ─────────────────────────
	// RunBatchWithProgress threads a per-item progress callback through
	// to the batch function so the build pipeline can report progress
	// during long-running hook execution (e.g., Node worker pool IPC).

	Describe("Batch progress callback (issue #686)", func() {
		It("progress callback fires once per completed item with correct total", func() {
			registry := plugin.NewHookRegistry()

			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			batchFn := func(_ context.Context, ps []interface{}, onProgress plugin.BatchProgressFunc) ([]interface{}, error) {
				out := make([]interface{}, len(ps))
				for i, p := range ps {
					out[i] = p.(string) + "-done"
					onProgress(i+1, len(ps))
				}
				return out, nil
			}
			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, batchFn, 50)

			var updates [][]int
			progress := func(completed, total int) {
				updates = append(updates, []int{completed, total})
			}

			results, err := registry.RunBatchWithProgress(plugin.OnPageRendered, []interface{}{"a", "b", "c"}, progress)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(Equal([]interface{}{"a-done", "b-done", "c-done"}),
				"batch results must be correct regardless of progress callback")
			Expect(updates).To(HaveLen(3),
				"progress callback must fire once per completed item")
			Expect(updates[0]).To(Equal([]int{1, 3}))
			Expect(updates[1]).To(Equal([]int{2, 3}))
			Expect(updates[2]).To(Equal([]int{3, 3}))
		})

		It("nil progress callback does not panic", func() {
			registry := plugin.NewHookRegistry()

			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			batchFn := func(_ context.Context, ps []interface{}, onProgress plugin.BatchProgressFunc) ([]interface{}, error) {
				out := make([]interface{}, len(ps))
				for i, p := range ps {
					out[i] = p
					onProgress(i+1, len(ps))
				}
				return out, nil
			}
			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, batchFn, 50)

			Expect(func() {
				_, _ = registry.RunBatchWithProgress(plugin.OnPageRendered, []interface{}{"a"}, nil)
			}).NotTo(Panic(),
				"nil progress callback must be safe — RunBatchWithProgress must "+
					"supply a no-op when caller passes nil")
		})

		It("progress completed count is monotonically increasing", func() {
			registry := plugin.NewHookRegistry()

			singleFn := func(_ context.Context, p interface{}) (interface{}, error) {
				return p, nil
			}
			batchFn := func(_ context.Context, ps []interface{}, onProgress plugin.BatchProgressFunc) ([]interface{}, error) {
				out := make([]interface{}, len(ps))
				for i, p := range ps {
					out[i] = p
					onProgress(i+1, len(ps))
				}
				return out, nil
			}
			registry.RegisterBatchWithPriority(plugin.OnPageRendered, singleFn, batchFn, 50)

			var counts []int
			progress := func(completed, total int) {
				counts = append(counts, completed)
			}

			_, err := registry.RunBatchWithProgress(plugin.OnPageRendered,
				[]interface{}{"a", "b", "c", "d", "e"}, progress)
			Expect(err).NotTo(HaveOccurred())

			for i := 1; i < len(counts); i++ {
				Expect(counts[i]).To(BeNumerically(">=", counts[i-1]),
					"completed count must be monotonically increasing — "+
						"concurrent workers may report out of order but the "+
						"atomic counter must never decrease")
			}
		})

		It("per-item fallback also fires progress callback", func() {
			registry := plugin.NewHookRegistry()

			registry.Register(plugin.OnPageRendered, func(_ context.Context, p interface{}) (interface{}, error) {
				return p.(string) + "-ok", nil
			})

			var updates [][]int
			progress := func(completed, total int) {
				updates = append(updates, []int{completed, total})
			}

			results, err := registry.RunBatchWithProgress(plugin.OnPageRendered,
				[]interface{}{"x", "y"}, progress)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(Equal([]interface{}{"x-ok", "y-ok"}))
			Expect(updates).To(HaveLen(2),
				"per-item fallback must also fire progress callback after each item")
			Expect(updates[0][0]).To(Equal(1))
			Expect(updates[1][0]).To(Equal(2))
		})
	})
})
