// Fixture: top-level module code writes to process.stdout during import.
// After stdout isolation (#968), this write must be redirected to stderr
// so the registration handshake still succeeds.
export const runtime = "node";

process.stdout.write("module initialization noise\n");

export default function(alloy) {
    alloy.filter("cleanFilter", (input) => String(input) + "-processed");
}
