export const runtime = "node";

export default function(alloy) {
  let lcss;

  alloy.hook("onPageRendered", { priority: 90, pages: true, pageFields: ["html"] }, async (html) => {
    if (typeof html !== 'string') return html;
    if (!/<template\s+shadow/.test(html)) return html;

    if (!lcss) {
      lcss = await import('lightningcss');
    }

    return html.replace(
      /(<template\s+shadowroot[^>]*>)([\s\S]*?)(<\/template>)/g,
      (full, open, inner, close) => {
        const minified = inner.replace(/<style>([\s\S]*?)<\/style>/g, (_, css) => {
          try {
            const { code } = lcss.transform({
              filename: 'shadow.css',
              code: Buffer.from(css),
              minify: true,
              include: lcss.Features.Nesting,
              errorRecovery: true,
            });
            return `<style>${code.toString()}</style>`;
          } catch {
            return `<style>${css}</style>`;
          }
        });
        return open + minified + close;
      }
    );
  });
}
