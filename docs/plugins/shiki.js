export const runtime = "node";

export default function(alloy) {
  let highlighter;

  async function ensureShiki() {
    if (highlighter) return;
    const shiki = await import('shiki');
    highlighter = await shiki.createHighlighter({
      themes: ['github-dark', 'github-light'],
      langs: ['rust', 'go', 'typescript', 'javascript', 'html', 'css',
              'yaml', 'toml', 'json', 'bash', 'shell', 'markdown'],
    });
  }

  function decodeHtmlEntities(s) {
    return s
      .replace(/&#123;/g, '{')
      .replace(/&#125;/g, '}')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&amp;/g, '&');
  }

  alloy.hook("onPageRendered", { priority: 10, pages: true, pageFields: ["html"] }, async (html) => {
    if (typeof html !== 'string') return html;
    if (!/<alloy-code/.test(html)) return html;

    await ensureShiki();

    const re = /<alloy-code([^>]*)>([\s\S]*?)<\/alloy-code>/g;
    const matches = [...html.matchAll(re)];

    for (const m of matches) {
      const [full, attrs, content] = m;
      const langMatch = attrs.match(/language="([^"]+)"/);
      const raw = decodeHtmlEntities(content.trim());

      if (langMatch) {
        const lang = langMatch[1];
        try {
          const loadedLangs = highlighter.getLoadedLanguages();
          if (!loadedLangs.includes(lang)) {
            await highlighter.loadLanguage(lang);
          }
          let highlighted = highlighter.codeToHtml(raw, {
            lang,
            themes: { light: 'github-light', dark: 'github-dark' },
            defaultColor: false,
          });
          html = html.replace(full, `<alloy-code${attrs}>${highlighted}</alloy-code>`);
          continue;
        } catch {
          // language not supported — fall through to plain wrap
        }
      }

      html = html.replace(full, `<alloy-code${attrs}><pre><code>${content.trim()}</code></pre></alloy-code>`);
    }

    return html;
  });
}
