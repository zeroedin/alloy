// Tier 3 Node plugin — needs npm packages
export const runtime = "node";
import postcss from 'postcss';
import cssnano from 'cssnano';

export default function(alloy) {
    alloy.on("onAssetProcess", async (file) => {
        if (file.path.endsWith('.css')) {
            const result = await postcss([cssnano]).process(file.content, { from: file.path });
            return { ...file, content: result.css };
        }
        return file;
    });
}
