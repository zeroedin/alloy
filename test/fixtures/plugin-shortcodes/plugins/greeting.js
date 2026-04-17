// Plugin that registers a shortcode via alloy.shortcode()
// Used by integration tests to verify the full pipeline:
//   plugin discovery → LoadPlugins → CallShortcode → template rendering
export default function(alloy) {
    alloy.shortcode("greeting", (args) => {
        const name = args[0] || "World";
        return `<p class="greeting">Hello, ${name}!</p>`;
    });
}
