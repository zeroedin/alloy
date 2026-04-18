// Plugin that hooks onContentTransformed to verify payload shape.
// If the payload contains <!DOCTYPE or <head>, the hook fired AFTER
// layout rendering — which violates the spec.
export default function(alloy) {
    alloy.hook("onContentTransformed", (html) => {
        if (html.includes("<!DOCTYPE") || html.includes("<head>")) {
            // Signal the violation by injecting a marker the test can detect
            return "HOOK_FIRED_AFTER_LAYOUT:" + html;
        }
        return html;
    });
}
