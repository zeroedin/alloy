// ESM-only plugin — uses import statement that eval() cannot handle.
// This fixture proves the bridge uses import() instead of eval().
export const runtime = "node";

import { basename } from "node:path";

export default function(alloy) {
  alloy.filter("baseName", (input) => basename(String(input)));
}
