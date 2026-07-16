// Fixture: plugin source handler that throws an error.
// Tests that source handler errors propagate through the bridge.
export const runtime = "node";

export default function(alloy) {
  alloy.source("failing-source", async () => {
    throw new Error("CMS API returned 503");
  });
}
