// Fixture: filter that calls process.stdout.write() directly.
// After stdout isolation (#968), this write must be redirected to stderr
// so it cannot corrupt the bridge protocol. The filter must still return
// the correct result.
export const runtime = "node";

export default function(alloy) {
    alloy.filter("noisyFilter", (input) => {
        process.stdout.write("garbage from filter\n");
        return String(input).toUpperCase();
    });
}
