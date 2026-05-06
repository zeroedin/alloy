// QuickJS plugin that registers a hook that MODIFIES the payload.
// Used by tests to prove hook bridging — the pass-through hooks.js
// can't distinguish "hook ran" from "no hooks registered."
export default function(alloy) {
    alloy.hook("onContentTransformed", { pages: true, pageFields: ["*"] }, function(content) {
        return content + "<!-- hook-modified -->";
    });
}
