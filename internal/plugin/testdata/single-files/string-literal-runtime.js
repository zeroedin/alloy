export default function(alloy) {
    const msg = 'was runtime = "node" before migration';
    alloy.filter("stringTest", (input) => msg + String(input).trim());
}
