// Plain JS plugin — no runtime export, runs on QuickJS (Tier 2)
export default function(alloy) {
    alloy.filter("wordCount", (s) => s.split(/\s+/).filter(w => w.length > 0).length);
    alloy.shortcode("greeting", (name) => `<p>Hello, ${name}!</p>`);
}
