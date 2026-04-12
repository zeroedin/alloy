// Plain JS plugin — no runtime export, runs on QuickJS (Tier 2)
export default function(alloy) {
    alloy.filter("exclaim", (s) => s + "!");
}
