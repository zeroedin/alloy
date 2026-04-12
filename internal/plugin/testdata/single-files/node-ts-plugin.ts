// TypeScript plugin that requires Node runtime (Tier 3)
export const runtime = "node";

export default function(alloy: any) {
    alloy.on("onContentTransformed", async (payload: any) => {
        return payload;
    });
}
