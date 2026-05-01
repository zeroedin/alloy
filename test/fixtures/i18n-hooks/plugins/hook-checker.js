// Plugin that hooks onContentTransformed to verify payload shape.
// If the payload contains <!DOCTYPE or <head>, the hook fired AFTER
// layout rendering — which violates the spec.
export default function(alloy) {
    alloy.hook("onContentTransformed", (page) => {
        const html = typeof page === 'string' ? page : page.html;
        if (html.includes("<!DOCTYPE") || html.includes("<head>")) {
            if (typeof page === 'string') return "HOOK_FIRED_AFTER_LAYOUT:" + html;
            page.html = "HOOK_FIRED_AFTER_LAYOUT:" + html;
            return page;
        }
        return page;
    });
}
