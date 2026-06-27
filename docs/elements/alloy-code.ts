import { LitElement, html, css } from 'lit';

export class AlloyCode extends LitElement {
  static properties = {
    lang: { type: String },
    filename: { type: String },
    _codeText: { state: true },
  };

  declare lang: string;
  declare filename: string;
  declare _codeText: string;

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
  `;

  constructor() {
    super();
    this.lang = '';
    this.filename = '';
    this._codeText = '';
  }

  private _syncCodeText(): void {
    const slot = this.shadowRoot?.querySelector('slot');
    const nodes = slot?.assignedNodes({ flatten: true }) ?? [];
    this._codeText = nodes.map(n => n.textContent).join('').trim();
  }

  protected firstUpdated(): void {
    this._syncCodeText();
  }

  protected render() {
    return html`
      ${this.filename ? html`
        <div class="header">
          <span class="filename">${this.filename}</span>
          <wa-copy-button .value=${this._codeText} copy-label="Copy" success-label="Copied!"></wa-copy-button>
        </div>
      ` : html`
        <wa-copy-button .value=${this._codeText} copy-label="Copy" success-label="Copied!" style="position:absolute;top:0.5rem;right:0.5rem;z-index:1"></wa-copy-button>
      `}
      <div class="code-container">
        <slot @slotchange=${this._syncCodeText}></slot>
      </div>
    `;
  }
}

customElements.define('alloy-code', AlloyCode);
