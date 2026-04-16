// Plugin that registers a filter the simulator cannot pattern-match.
// Proves CallFilter must execute actual JS, not just recognize known patterns.
export default function(alloy) {
    alloy.filter("uppercase", (input) => String(input).toUpperCase());
}
