export const runtime = "node";

export default function(alloy) {
  let renderString;

  async function ensureLoaded() {
    if (renderString) return;

    await import('@lit-labs/ssr/lib/install-global-dom-shim.js');

    if (!globalThis.__ceDefinePatched) {
      globalThis.__ceDefinePatched = true;
      const ceObj = globalThis.customElements;
      const _origDefine = ceObj.define.bind(ceObj);
      ceObj.define = function(name, ctor, options) {
        if (ceObj.get(name)) return;
        return _origDefine(name, ctor, options);
      };
      let _ce = globalThis.customElements;
      Object.defineProperty(globalThis, 'customElements', {
        get() { return _ce; },
        set(v) { _ce = v; },
        configurable: true,
      });
    }

    await import('../docs/elements/alloy-code.js');
    await import('@awesome.me/webawesome/dist/ssr/all.js');
    const mod = await import('@awesome.me/webawesome/dist/ssr/render-string.js');
    renderString = mod.renderString;
  }

  alloy.hook("onPageRendered", { priority: 50, pages: true, pageFields: ["html"] }, async (html) => {
    if (typeof html !== 'string') return html;

    if (/<wa-/.test(html) || /<alloy-code/.test(html)) {
      await ensureLoaded();
      try {
        html = renderString(html);
      } catch (e) {
        const m = html.match(/<title>([^<]*)<\/title>/);
        const page = m ? m[1].trim() : '(unknown page)';
        console.error(`[lit-ssr] SSR failed on "${page}": ${e.message}`);
      }
    }

    return html;
  });
}
