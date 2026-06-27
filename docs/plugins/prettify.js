export const runtime = "node";

export default function(alloy) {
  let processor;

  alloy.hook("onPageRendered", { priority: 100, pages: true, pageFields: ["html"] }, async (html) => {
    if (typeof html !== 'string') return html;

    if (!processor) {
      const { unified } = await import('unified');
      const rehypeParse = (await import('rehype-parse')).default;
      const rehypeFormat = (await import('rehype-format')).default;
      const rehypeStringify = (await import('rehype-stringify')).default;

      processor = unified()
        .use(rehypeParse)
        .use(rehypeFormat, { indent: 2 })
        .use(rehypeStringify, { allowDangerousHtml: true });
    }

    try {
      const result = await processor.process(html);
      return String(result);
    } catch (e) {
      const m = html.match(/<title>([^<]*)<\/title>/);
      const page = m ? m[1].trim() : '(unknown page)';
      console.error(`[prettify] failed on "${page}": ${e.message}`);
      return html;
    }
  });
}
