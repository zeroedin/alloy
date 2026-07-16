// Fixture: plugin that registers a data source handler via alloy.source().
// Returns a static array of posts for testing the source registration
// and invocation path through the Node bridge.
export const runtime = "node";

export default function(alloy) {
  alloy.source("test-source", async (config) => {
    return [
      { title: "Post 1", slug: "post-1" },
      { title: "Post 2", slug: "post-2" },
    ];
  });
}
