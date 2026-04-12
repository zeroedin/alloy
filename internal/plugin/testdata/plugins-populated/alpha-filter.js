// Tier 2 QuickJS plugin — no runtime: "node" export
export default function(alloy) {
    alloy.filter("wordCount", (content) => {
        return content.split(/\s+/).filter(w => w.length > 0).length;
    });
}
