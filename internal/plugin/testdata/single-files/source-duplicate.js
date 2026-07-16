// Fixture: plugin that registers the same source name twice.
// Tests that duplicate source registration produces a warning
// (same pattern as duplicate hook registration).
export const runtime = "node";

export default function(alloy) {
  alloy.source("dup-source", async () => { return [1]; });
  alloy.source("dup-source", async () => { return [2]; });
}
