// Fixture: plugin source handler that intentionally blocks for a long time.
// Tests that CallSource applies a timeout and does not hang the build.
export const runtime = "node";

export default function(alloy) {
  alloy.source("slow-source", async () => {
    // Block for 60 seconds — well beyond any reasonable plugin timeout.
    // If CallSource has no timeout, this hangs the build indefinitely.
    await new Promise(resolve => setTimeout(resolve, 60000));
    return [{ title: "Should never reach here" }];
  });
}
