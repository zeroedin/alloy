// Plugin that hooks onContentTransformed to verify payload shape.
// If the payload contains <!DOCTYPE or <head>, the hook fired AFTER
// layout rendering — which violates the spec.
export default function(alloy) {
    alloy.hook("onContentTransformed", (page) => {
        if (page.html.includes("<!DOCTYPE") || page.html.includes("<head>")) {
            page.html = "HOOK_FIRED_AFTER_LAYOUT:" + page.html;
        }
        return page;
    });
}
