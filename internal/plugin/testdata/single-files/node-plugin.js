// JS plugin that requires Node runtime (Tier 3)
export const runtime = "node";
import fs from 'fs';

export default function(alloy) {
    alloy.on("onBuildComplete", {}, async (payload) => {
        return payload;
    });
}
