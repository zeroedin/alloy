export const runtime = "node";

export default function(alloy) {
  let ready = false;
  let litRender, unsafeHTML, LitElementRenderer, trimOuterMarkers, lcss;

  // Shared state across all pages in a build
  const specifierMap = new Map();      // specifier -> processed CSS string
  const styleIdentityMap = new WeakMap(); // CSSResult -> specifier name

  async function ensureLoaded() {
    if (ready) return;

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

    await import('../elements/alloy-code.js');
    await import('@awesome.me/webawesome/dist/ssr/all.js');
    await import('@awesome.me/webawesome/dist/components/copy-button/copy-button.js');

    const litMod = await import('@lit-labs/ssr');
    litRender = litMod.render;

    const uhMod = await import('lit/directives/unsafe-html.js');
    unsafeHTML = uhMod.unsafeHTML;

    const rendererMod = await import('@lit-labs/ssr/lib/lit-element-renderer.js');
    LitElementRenderer = rendererMod.LitElementRenderer;

    const trimMod = await import('@awesome.me/webawesome/dist/ssr/trim-outer-markers.js');
    trimOuterMarkers = trimMod.trimOuterMarkers;

    lcss = await import('lightningcss');

    ready = true;
  }

  function processCSS(cssText) {
    try {
      const { code } = lcss.transform({
        filename: 'constructed-stylesheet.css',
        code: Buffer.from(cssText),
        minify: true,
        include: lcss.Features.Nesting,
        errorRecovery: true,
      });
      return code.toString()
        .replaceAll('color-scheme:normal', 'color-scheme:inherit');
    } catch {
      return cssText;
    }
  }

  function createDeduplicatingRenderer() {
    // Create a subclass that intercepts style rendering
    class DeduplicatingRenderer extends LitElementRenderer {
      #specifiers = [];

      connectedCallback() {
        super.connectedCallback();
        const styles = this.element.constructor.elementStyles ?? [];

        // Build a list of unnamed styles (no .specifier property)
        const unnamed = styles.filter(s =>
          !styleIdentityMap.has(s) && !s.specifier
        );

        for (const style of styles) {
          // Determine specifier: explicit .specifier, or derive from tag name
          const specifier = styleIdentityMap.get(style)
            ?? style.specifier
            ?? (unnamed.length === 1
              ? this.tagName.toLowerCase()
              : `${this.tagName.toLowerCase()}-${unnamed.indexOf(style)}`);

          styleIdentityMap.set(style, specifier);
          this.#specifiers.push(specifier);

          if (!specifierMap.has(specifier)) {
            specifierMap.set(specifier, processCSS(style.cssText));
          }
        }
      }

      renderAttributes() {
        const result = super.renderAttributes();
        if (this.#specifiers.length > 0) {
          result.push(
            ` shadowrootadoptedstylesheets="${this.#specifiers.join(' ')}"`
          );
        }
        return result;
      }

      renderShadow(renderInfo) {
        const result = [];

        // Emit marker comment that post-processing will lift to the <template> tag
        if (this.#specifiers.length > 0) {
          result.push(`<!--@adopted:${this.#specifiers.join(' ')}-->`);
        }

        // Render the element's template (no <style> blocks — styles are deduped)
        const parent = super.renderShadow(renderInfo);
        if (parent) {
          // Filter out the style thunk from parent's output.
          // LitElementRenderer emits [styleThunk, templateThunk].
          // We skip the style thunk (index 0) and keep the template.
          for (const item of parent) {
            result.push(item);
          }
        }

        return result;
      }
    }

    return DeduplicatingRenderer;
  }

  function postProcess(html) {
    // 1. Lift <!--@adopted:...--> markers onto their parent <template> tags
    html = html.replace(
      /(<template\s+shadowroot(?:mode)?="open"[^>]*)(>)\s*<!--@adopted:([\w\s-]+)-->/g,
      (_, templateOpen, close, specifiers) => {
        return `${templateOpen} shadowrootadoptedstylesheets="${specifiers.trim()}"${close}`;
      }
    );

    // 2. Strip inline <style> blocks from inside <template shadowroot> since
    //    styles are now in shared <style type="module"> blocks.
    //    Only strip if the template has shadowrootadoptedstylesheets.
    html = html.replace(
      /(<template\s+shadowroot[^>]*shadowrootadoptedstylesheets[^>]*>)([\s\S]*?)(<\/template>)/g,
      (_, open, inner, close) => {
        const stripped = inner.replace(/<style>[\s\S]*?<\/style>/g, '');
        return open + stripped + close;
      }
    );

    // 3. Inject shared <style type="module"> blocks before </head>
    if (specifierMap.size > 0) {
      const styleBlocks = [];
      for (const [specifier, css] of specifierMap) {
        styleBlocks.push(
          `<style type="module" specifier="${specifier}">${css}</style>`
        );
      }
      html = html.replace('</head>', styleBlocks.join('\n') + '\n</head>');
    }

    return html;
  }

  alloy.hook("onPageRendered", { priority: 50, pages: true, pageFields: ["html"] }, async (html) => {
    if (typeof html !== 'string') return html;
    if (!/<wa-/.test(html) && !/<alloy-code/.test(html)) return html;

    await ensureLoaded();

    try {
      const Renderer = createDeduplicatingRenderer();
      const iterator = litRender(unsafeHTML(html), {
        elementRenderers: [Renderer],
      });
      let result = [];
      for (const chunk of iterator) {
        result.push(chunk);
      }
      html = trimOuterMarkers(result.join(''));
      html = postProcess(html);
    } catch (e) {
      const m = html.match(/<title>([^<]*)<\/title>/);
      const page = m ? m[1].trim() : '(unknown page)';
      console.error(`[lit-ssr] SSR failed on "${page}": ${e.message}`);
    }

    return html;
  });
}
