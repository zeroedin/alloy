// Plugin that registers lifecycle hooks via both alloy.hook() and alloy.on()
export default function(alloy) {
    alloy.hook("onContentTransformed", { pages: true, pageFields: ["*"] }, function(content) {
        return content;
    });
    alloy.on("onPageRendered", {}, function(page) {
        return page;
    });
}
