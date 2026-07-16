package fetch_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/fetch"
)

var _ = Describe("Fetch", func() {

	// ── REST fetcher ───────────────────────────────────────────────────

	Describe("FetchREST", func() {
		var restServer *httptest.Server

		BeforeEach(func() {
			restServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":    1,
					"title": "test post",
					"body":  "hello from httptest",
				})
			}))
		})

		AfterEach(func() {
			restServer.Close()
		})

		It("returns data from URL", func() {
			data, err := fetch.FetchREST(restServer.URL + "/posts/1")
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeNil())
		})

		It("returns error on fetch failure", func() {
			_, err := fetch.FetchREST("https://invalid.example.test/404")
			Expect(err).To(HaveOccurred())
			// The error must describe the network/HTTP failure, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("fetch"),
					ContainSubstring("request"),
					ContainSubstring("HTTP"),
					ContainSubstring("connection"),
					ContainSubstring("dial"),
				),
				"error should indicate a network or HTTP failure",
			)
		})
	})

	// ── GraphQL fetcher ────────────────────────────────────────────────

	Describe("FetchGraphQL", func() {
		var gqlServer *httptest.Server

		BeforeEach(func() {
			gqlServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"data": map[string]interface{}{
						"products": []interface{}{
							map[string]interface{}{"id": "1", "name": "Widget"},
							map[string]interface{}{"id": "2", "name": "Gadget"},
						},
					},
				})
			}))
		})

		AfterEach(func() {
			gqlServer.Close()
		})

		It("sends query and returns unwrapped data", func() {
			data, err := fetch.FetchGraphQL(
				gqlServer.URL+"/graphql",
				`{ products { id name } }`,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeNil())
		})

		It("returns error on failure", func() {
			_, err := fetch.FetchGraphQL("https://invalid.example.test/graphql", "{bad}")
			Expect(err).To(HaveOccurred())
			// The error must describe the network/GraphQL failure, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("fetch"),
					ContainSubstring("graphql"),
					ContainSubstring("request"),
					ContainSubstring("connection"),
					ContainSubstring("dial"),
				),
				"error should indicate a network or GraphQL failure",
			)
		})
	})

	// ── Caching ────────────────────────────────────────────────────────

	Describe("Caching", func() {
		var cacheDir string

		BeforeEach(func() {
			cacheDir = GinkgoT().TempDir()
		})

		It("GetCached returns false when no cache exists, true after saving", func() {
			_, found := fetch.GetCached("roundtrip-key", cacheDir, 3600)
			Expect(found).To(BeFalse())

			// After successfully saving, the same key must be retrievable
			err := fetch.SaveCache("roundtrip-key", cacheDir, map[string]string{"k": "v"})
			Expect(err).NotTo(HaveOccurred())

			data, found := fetch.GetCached("roundtrip-key", cacheDir, 3600)
			Expect(found).To(BeTrue(), "cached key should be found after SaveCache")
			Expect(data).NotTo(BeNil())
		})

		It("SaveCache stores data without error", func() {
			err := fetch.SaveCache("test-key", cacheDir, map[string]string{"k": "v"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Response parsing ──────────────────────────────────────────────

	Describe("Response parsing", func() {
		It("ParseXML parses XML response into map", func() {
			xml := []byte(`<root><name>Test</name><value>42</value></root>`)
			result, err := fetch.ParseXML(xml)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKey("name"))
		})

		It("ParseCSVResponse parses CSV response into rows", func() {
			csv := []byte("name,role\nAlice,engineer\nBob,designer\n")
			rows, err := fetch.ParseCSVResponse(csv)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(2))
			Expect(rows[0]).To(HaveKeyWithValue("name", "Alice"))
		})

		It("UnwrapGraphQLData extracts data envelope", func() {
			raw := map[string]interface{}{
				"data": map[string]interface{}{
					"products": []interface{}{"a", "b"},
				},
				"errors": nil,
			}
			result, err := fetch.UnwrapGraphQLData(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			dataMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(dataMap).To(HaveKey("products"))
		})
	})

	// ── Cache configuration ───────────────────────────────────────────

	Describe("Cache configuration", func() {
		var cacheDir string

		BeforeEach(func() {
			cacheDir = GinkgoT().TempDir()
		})

		It("CacheDir returns .alloy/fetch-cache/ under project root", func() {
			dir := fetch.CacheDir("/my/project")
			Expect(dir).To(Equal("/my/project/.alloy/fetch-cache/"))
		})

		It("expired cache is not returned in build mode", func() {
			_, found, err := fetch.GetCachedWithTTL("expired-key", cacheDir, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse(),
				"expired cache entry must not be returned")
		})

		It("--refetch flag bypasses cache", func() {
			// With refetch=true, cache should be ignored and a fresh fetch attempted
			_, err := fetch.FetchRESTWithRefetch("https://example.com/api", cacheDir, true)
			Expect(err).To(HaveOccurred(),
				"stub must fail, proving refetch path is exercised")
			Expect(err.Error()).NotTo(Equal("not implemented"),
				"error must be fetch-specific, not generic stub")
		})
	})

	// ── Plugin data sources ──────────────────────────────────────────

	Describe("Plugin data sources", func() {
		BeforeEach(func() {
			fetch.ResetPluginSources()
		})

		It("config parses type: plugin with plugin name and cache TTL", func() {
			// This tests that the config system recognizes plugin sources
			sourceCfg := map[string]interface{}{
				"type":   "plugin",
				"plugin": "custom-cms",
				"cache":  3600,
			}
			Expect(sourceCfg["type"]).To(Equal("plugin"))
			Expect(sourceCfg["plugin"]).To(Equal("custom-cms"))

			// The actual fetch should invoke the registered handler
			_, err := fetch.FetchPluginSource("custom-cms", sourceCfg)
			Expect(err).To(HaveOccurred(),
				"stub must fail, proving plugin source path is exercised")
			Expect(err.Error()).NotTo(Equal("not implemented"),
				"error must be source-specific, not generic stub")
		})

		It("plugin source handler registration stores the handler", func() {
			handler := func(config map[string]interface{}) (interface{}, error) {
				return nil, nil
			}
			fetch.RegisterPluginSource("test-source", handler)

			sources := fetch.RegisteredPluginSources()
			Expect(sources).To(ContainElement("test-source"),
				"registered plugin source must appear in the list")
		})

		It("FetchPluginSource invokes the registered handler", func() {
			called := false
			fetch.RegisterPluginSource("tracker", func(config map[string]interface{}) (interface{}, error) {
				called = true
				return map[string]string{"status": "ok"}, nil
			})

			result, err := fetch.FetchPluginSource("tracker", map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue(), "handler must be invoked")
			Expect(result).NotTo(BeNil())
		})

		It("plugin source data is injectable into site.data namespace", func() {
			// Verify the plugin source returns data in a format compatible with site.data
			_, err := fetch.FetchPluginSource("cms-source", map[string]interface{}{
				"as": "cms_posts",
			})
			Expect(err).To(HaveOccurred(),
				"unregistered source must produce an error")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("cms-source"),
				ContainSubstring("not registered"),
				ContainSubstring("not found"),
			), "error must identify the missing source handler")
		})

		// ── Plugin source handler edge cases (issue #979) ──────────────

		It("ResetPluginSources clears all registered handlers", func() {
			fetch.RegisterPluginSource("src-a", func(config map[string]interface{}) (interface{}, error) {
				return "data-a", nil
			})
			fetch.RegisterPluginSource("src-b", func(config map[string]interface{}) (interface{}, error) {
				return "data-b", nil
			})
			Expect(fetch.RegisteredPluginSources()).To(HaveLen(2))

			fetch.ResetPluginSources()
			Expect(fetch.RegisteredPluginSources()).To(BeEmpty(),
				"ResetPluginSources must remove all registered handlers")

			_, err := fetch.FetchPluginSource("src-a", nil)
			Expect(err).To(HaveOccurred(),
				"previously registered handler must not be callable after reset")
			Expect(err.Error()).To(ContainSubstring("not registered"),
				"error must indicate the source is not registered")
		})

		It("FetchPluginSource passes config map to handler unchanged", func() {
			var receivedConfig map[string]interface{}
			fetch.RegisterPluginSource("config-reader", func(config map[string]interface{}) (interface{}, error) {
				receivedConfig = config
				return "ok", nil
			})

			inputConfig := map[string]interface{}{
				"type":   "plugin",
				"plugin": "config-reader",
				"cache":  float64(3600),
				"as":     "blog",
			}
			_, err := fetch.FetchPluginSource("config-reader", inputConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedConfig).To(Equal(inputConfig),
				"handler must receive the exact config map passed to FetchPluginSource")
		})

		It("FetchPluginSource propagates handler errors", func() {
			fetch.RegisterPluginSource("failing-source", func(config map[string]interface{}) (interface{}, error) {
				return nil, fmt.Errorf("API returned 503")
			})

			_, err := fetch.FetchPluginSource("failing-source", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("API returned 503"),
				"handler error message must propagate to the caller unchanged")
		})

		It("FetchPluginSource returns handler data unchanged for various types", func() {
			// Array data
			fetch.RegisterPluginSource("array-src", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{"post1", "post2", "post3"}, nil
			})
			result, err := fetch.FetchPluginSource("array-src", nil)
			Expect(err).NotTo(HaveOccurred())
			arr, ok := result.([]interface{})
			Expect(ok).To(BeTrue(), "result must be a slice")
			Expect(arr).To(HaveLen(3))
			Expect(arr[0]).To(Equal("post1"))

			// Nested map data
			fetch.RegisterPluginSource("map-src", func(config map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{
					"posts": []interface{}{
						map[string]interface{}{"title": "First", "slug": "first"},
					},
				}, nil
			})
			result, err = fetch.FetchPluginSource("map-src", nil)
			Expect(err).NotTo(HaveOccurred())
			m, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "result must be a map")
			posts, ok := m["posts"].([]interface{})
			Expect(ok).To(BeTrue(), "posts must be a slice")
			Expect(posts).To(HaveLen(1))
			first, ok := posts[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(first["title"]).To(Equal("First"))
		})

		It("RegisterPluginSource overwrites previous handler for same name", func() {
			fetch.RegisterPluginSource("overwrite-test", func(config map[string]interface{}) (interface{}, error) {
				return "first", nil
			})
			fetch.RegisterPluginSource("overwrite-test", func(config map[string]interface{}) (interface{}, error) {
				return "second", nil
			})

			result, err := fetch.FetchPluginSource("overwrite-test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("second"),
				"last registered handler must win when the same name is registered twice")
		})
	})
})
