// Plugin that reverses a string — uses JS logic that cannot be pattern-matched
// by simulateJSFilter (no .toUpperCase, .split+.length, .trim, etc.)
export default function(alloy) {
    alloy.filter("reverse", (input) => String(input).split("").reverse().join(""));
}
