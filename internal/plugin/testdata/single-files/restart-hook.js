// Fixture: plugin that registers a filter and a hook for restart tests.
// After Restart(), a fresh bridge must be able to call both.
export const runtime = "node";

export default function(alloy) {
    alloy.filter("restartUpper", (input) => String(input).toUpperCase());
    alloy.hook("onBuildComplete", { data: true }, (payload) => {
        return { ...payload, restarted: true };
    });
}
