// Previously used runtime = "node" but switched to QuickJS for performance
export default function(alloy) {
    alloy.filter("commentTest", (input) => String(input).trim());
}
