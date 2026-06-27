import { LitElement, html, css } from 'lit';

export class AlloyCode extends LitElement {
  static properties = {
    lang: { type: String },
    filename: { type: String },
  };

  declare lang: string;
  declare filename: string;

  static styles = css`
    :host {
      display: block;
      position: relative;
      margin-bottom: var(--space-lg, 1.5rem);
    }

    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0.4rem 1rem;
      background: var(--alloy-code-header-bg, rgba(255, 255, 255, 0.05));
      border-radius: var(--radius-lg, 6px) var(--radius-lg, 6px) 0 0;
      border-bottom: 1px solid var(--alloy-code-border, rgba(255, 255, 255, 0.1));
    }

    .filename {
      font-family: var(--font-mono, monospace);
      font-size: 0.8rem;
      color: var(--color-text-muted, #a1a1aa);
    }

    .copy-btn {
      appearance: none;
      border: none;
      background: transparent;
      color: var(--color-text-muted, #a1a1aa);
      cursor: pointer;
      padding: 0.25rem;
      border-radius: var(--radius-sm, 3px);
      display: flex;
      align-items: center;
      transition: color 0.15s ease;

      &:hover {
        color: var(--color-text, #e4e4e7);
      }
    }

    .code-container {
      overflow-x: auto;
    }

    ::slotted(pre) {
      margin: 0;
      padding: var(--space-md, 1rem) var(--space-lg, 1.5rem);
      border-radius: var(--radius-lg, 6px);
      line-height: 1.5;
      font-size: 0.85rem;
      background: var(--alloy-code-bg, #24292e);
      color: var(--alloy-code-color, #e1e4e8);
    }

    :host([filename]) ::slotted(pre) {
      border-radius: 0 0 var(--radius-lg, 6px) var(--radius-lg, 6px);
    }

    ::slotted(pre) code {
      font-family: var(--font-mono, monospace);
    }

    .copied {
      color: var(--color-primary, #60a5fa);
    }
  `;

  constructor() {
    super();
    this.lang = '';
    this.filename = '';
  }

  private async copy(): Promise<void> {
    const slot = this.shadowRoot?.querySelector('slot');
    const nodes = slot?.assignedNodes({ flatten: true }) ?? [];
    const text = nodes.map(n => n.textContent).join('').trim();
    await navigator.clipboard.writeText(text);
    const btn = this.shadowRoot?.querySelector('.copy-btn');
    btn?.classList.add('copied');
    setTimeout(() => btn?.classList.remove('copied'), 1500);
  }

  private copyIcon() {
    return html`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>`;
  }

  protected render() {
    return html`
      ${this.filename ? html`
        <div class="header">
          <span class="filename">${this.filename}</span>
          <button class="copy-btn" @click=${this.copy} aria-label="Copy code">
            ${this.copyIcon()}
          </button>
        </div>
      ` : html`
        <button class="copy-btn" style="position:absolute;top:0.5rem;right:0.5rem;z-index:1" @click=${this.copy} aria-label="Copy code">
          ${this.copyIcon()}
        </button>
      `}
      <div class="code-container">
        <slot></slot>
      </div>
    `;
  }
}

customElements.define('alloy-code', AlloyCode);
