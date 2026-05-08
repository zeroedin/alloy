// Plugin that registers the same hook twice with different scopes (issue #544).
export default function(alloy) {
    alloy.hook("onContentTransformed", { pages: false }, function(content) {
        return content;
    });
    alloy.hook("onContentTransformed", { pages: true, pageFields: ["html"] }, function(content) {
        return content;
    });
}
