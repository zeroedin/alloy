// Minimal Node plugin — no external imports, uses only built-in APIs.
// Used by tests to verify Node plugin loading without npm dependencies.
export const runtime = "node";

export default function(alloy) {
    alloy.filter("nodeUpper", (input) => String(input).toUpperCase());
    alloy.hook("onContentTransformed", { pages: true, pageFields: ["*"] }, (html) => html + "<!-- node-plugin -->");
}
