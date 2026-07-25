export const runtime = "node";

export default function(alloy) {
  let processor;

  alloy.hook("onPageRendered", { priority: 100, pages: true, pageFields: ["html"] }, async (page) => {
    if (typeof page.html !== 'string') return page;
    if (!page.html.includes('<!DOCTYPE') && !page.html.includes('<html')) return page;

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
      const result = await processor.process(page.html);
      page.html = String(result);
      return page;
    } catch (e) {
      const m = page.html.match(/<title>([^<]*)<\/title>/);
      const title = m ? m[1].trim() : '(unknown page)';
      console.error(`[prettify] failed on "${title}": ${e.message}`);
      return page;
    }
  });
}
