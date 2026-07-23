import { LitElement, html, css, nothing } from 'lit';

export class AlloySearch extends LitElement {
  static properties = {
    site: { type: String },
  };

  declare site: string;

  private _fuse: any = null;
  private _selectedIndex = -1;
  private _debounceTimer: ReturnType<typeof setTimeout> | null = null;

  static styles = css`
    :host {
      position: relative;
      display: block;
    }

    form {
      display: flex;
      align-items: center;
      gap: var(--space-sm, 0.5rem);
      padding: 0.35rem 0.75rem;
      border-radius: 1.5rem;
      border: 1px solid var(--color-border, #e5e7eb);
      background: var(--color-surface, #f9fafb);
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }

    form:focus-within {
      border-color: var(--color-primary, #2563eb);
      box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary, #2563eb) 20%, transparent);
    }

    search {
      display: flex;
      align-items: center;
      gap: 0.35rem;
      flex: 1;
      position: relative;
    }

    svg {
      width: 0.875rem;
      height: 0.875rem;
      fill: var(--color-text-muted, #6b7280);
      flex-shrink: 0;
      opacity: 0.6;
    }

    input[type="search"] {
      border: none;
      background: transparent;
      outline: none;
      font-size: 0.85rem;
      font-family: inherit;
      color: var(--color-text, #1a1a2e);
      width: 10rem;
      padding: 0.15rem 0;
    }

    input[type="search"]::placeholder {
      color: var(--color-text-muted, #6b7280);
      opacity: 0.7;
    }

    input[type="search"]::-webkit-search-cancel-button {
      display: none;
    }

    kbd {
      position: absolute;
      right: 0;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-family: inherit;
      font-size: 0.7rem;
      font-weight: 600;
      min-width: 1.25rem;
      height: 1.25rem;
      padding: 0 0.3rem;
      border-radius: 3px;
      border: 1px solid var(--color-border, #e5e7eb);
      color: var(--color-text-muted, #6b7280);
      background: var(--color-bg, #fff);
      pointer-events: none;
      opacity: 0.7;
    }

    input:not(:placeholder-shown) ~ kbd {
      display: none;
    }

    button[type="submit"] {
      display: none;
    }

    [role="listbox"] {
      display: none;
      position: absolute;
      top: calc(100% + 4px);
      left: 0;
      width: max(100%, 20rem);
      max-height: 24rem;
      overflow-y: auto;
      background: var(--color-bg, #fff);
      border: 1px solid var(--color-border, #e5e7eb);
      border-radius: var(--radius-lg, 6px);
      box-shadow: 0 4px 24px rgba(0, 0, 0, 0.12);
      z-index: 1000;
    }

    [role="listbox"].active {
      display: block;
    }

    [role="listbox"]:empty {
      display: none;
    }

    [role="listbox"] a {
      display: block;
      padding: 0.5rem 0.75rem;
      text-decoration: none;
      color: var(--color-text, #1a1a2e);
      border-bottom: 1px solid var(--color-border, #e5e7eb);
    }

    [role="listbox"] a:last-child {
      border-bottom: none;
    }

    [role="listbox"] a:hover,
    [role="listbox"] a.selected {
      background: var(--color-primary, #2563eb);
      color: #fff;
    }

    [role="listbox"] a:hover h3,
    [role="listbox"] a.selected h3 {
      color: #fff;
    }

    [role="listbox"] h3 {
      margin: 0;
      font-size: 0.85rem;
      font-weight: 600;
      color: var(--color-text, #1a1a2e);
    }

    [role="listbox"] p {
      margin: 0.2rem 0 0;
      font-size: 0.75rem;
      line-height: 1.4;
      color: inherit;
      opacity: 0.8;
    }

    [role="status"] {
      padding: 0.5rem 0.75rem;
      font-size: 0.8rem;
      color: var(--color-text-muted, #6b7280);
    }
  `;

  constructor() {
    super();
    this.site = '';
  }

  protected firstUpdated(): void {
    this._setupAccessibility();
    this._loadSearchData();
    this._setupGlobalListeners();
  }

  private get _searchInput(): HTMLInputElement | null {
    return this.shadowRoot?.querySelector('input[type="search"]') ?? null;
  }

  private get _resultsContainer(): HTMLDivElement | null {
    return this.shadowRoot?.querySelector('[role="listbox"]') ?? null;
  }

  private _setupAccessibility(): void {
    const input = this._searchInput;
    const listbox = this._resultsContainer;
    if (!input || !listbox) return;

    const listboxId = `search-listbox-${Math.random().toString(36).substr(2, 9)}`;
    listbox.id = listboxId;
    input.setAttribute('aria-controls', listboxId);
  }

  private _setupGlobalListeners(): void {
    document.addEventListener('click', (event: Event) => {
      if (!this.contains(event.target as Node)) {
        this._clearResults();
      }
    });

    document.addEventListener('keydown', (event: KeyboardEvent) => {
      if (event.code === 'Escape') {
        this._clearResults();
        this._searchInput?.blur();
      }
      if (event.key === '/' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(
        (document.activeElement as HTMLElement)?.tagName
      )) {
        event.preventDefault();
        this._searchInput?.focus();
      }
    });
  }

  private async _loadSearchData(): Promise<void> {
    try {
      const response = await fetch('/search-index.json');
      if (!response.ok) {
        throw new Error(`Failed to load search index: ${response.status}`);
      }

      const searchData = await response.json();
      const { default: Fuse } = await import('fuse.js');

      this._fuse = new Fuse(searchData, {
        ignoreLocation: true,
        findAllMatches: true,
        includeScore: true,
        shouldSort: true,
        keys: [
          { name: 'title', weight: 5 },
          { name: 'description', weight: 2 },
          { name: 'body', weight: 1 },
          { name: 'section', weight: 0.5 },
        ],
        threshold: 0.1,
      });
    } catch (error) {
      console.warn('Search index could not be loaded, falling back to DuckDuckGo:', error);
    }
  }

  private _onFormSubmit = (event: Event): void => {
    if (this._fuse && this._searchInput?.value.trim()) {
      event.preventDefault();
    }
  };

  private _onSearchInput = (event: Event): void => {
    const query = (event.target as HTMLInputElement).value.trim();

    if (this._debounceTimer) {
      clearTimeout(this._debounceTimer);
    }

    this._debounceTimer = setTimeout(() => {
      this._performSearch(query);
    }, 150);
  };

  private _onKeyDown = (event: KeyboardEvent): void => {
    const container = this._resultsContainer;
    if (!container) return;

    const results = container.querySelectorAll('a[role="option"]');
    if (results.length === 0) return;

    switch (event.code) {
      case 'ArrowDown':
        event.preventDefault();
        this._selectedIndex = this._selectedIndex < results.length - 1
          ? this._selectedIndex + 1
          : 0;
        this._updateSelection(results);
        break;

      case 'ArrowUp':
        event.preventDefault();
        this._selectedIndex = this._selectedIndex > 0
          ? this._selectedIndex - 1
          : results.length - 1;
        this._updateSelection(results);
        break;

      case 'Enter':
        event.preventDefault();
        if (this._selectedIndex >= 0 && results[this._selectedIndex]) {
          (results[this._selectedIndex] as HTMLAnchorElement).click();
        }
        break;

      case 'Escape':
        event.preventDefault();
        this._clearResults();
        this._searchInput?.blur();
        break;
    }
  };

  private _updateSelection(results: NodeListOf<Element>): void {
    const input = this._searchInput;
    if (input) input.removeAttribute('aria-activedescendant');

    results.forEach((result, index) => {
      const isSelected = index === this._selectedIndex;
      result.classList.toggle('selected', isSelected);
      result.setAttribute('aria-selected', isSelected.toString());
      if (isSelected && input) {
        input.setAttribute('aria-activedescendant', result.id);
      }
    });
  }

  private _performSearch(query: string): void {
    if (!query || query.length < 2) {
      this._clearResults();
      return;
    }

    if (!this._fuse) {
      this._showMessage('Search index loading…');
      return;
    }

    const results = this._fuse.search(query);
    this._displayResults(results, query);
  }

  private _displayResults(results: any[], query: string): void {
    this._clearResults();
    this._selectedIndex = -1;

    const container = this._resultsContainer;
    if (!container) return;

    if (results.length === 0) {
      this._showMessage('No results found');
      return;
    }

    const limited = results.slice(0, 8);

    limited.forEach(({ item }: { item: any }, index: number) => {
      const link = document.createElement('a');
      link.href = item.link;
      link.setAttribute('role', 'option');
      link.setAttribute('aria-selected', 'false');
      link.id = `search-result-${index}`;

      const title = document.createElement('h3');
      title.textContent = item.title;
      link.appendChild(title);

      if (item.description) {
        const description = document.createElement('p');
        description.textContent = item.description;
        link.appendChild(description);
      }

      container.appendChild(link);
    });

    container.classList.add('active');
    this._searchInput?.setAttribute('aria-expanded', 'true');
  }

  private _showMessage(message: string): void {
    this._clearResults();
    const container = this._resultsContainer;
    if (!container) return;

    const el = document.createElement('div');
    el.setAttribute('role', 'status');
    el.textContent = message;
    container.appendChild(el);
    container.classList.add('active');
  }

  private _clearResults(): void {
    const container = this._resultsContainer;
    if (!container) return;

    container.innerHTML = '';
    container.classList.remove('active');
    this._selectedIndex = -1;
    this._searchInput?.setAttribute('aria-expanded', 'false');
    this._searchInput?.removeAttribute('aria-activedescendant');
  }

  protected render() {
    return html`
      <form action="https://duckduckgo.com/"
            method="get"
            target="_blank"
            @submit=${this._onFormSubmit}>
        ${this.site ? html`<input type="hidden" name="sites" .value=${this.site}>` : nothing}
        <search>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="11" cy="11" r="8" fill="none" stroke="currentColor" stroke-width="2"/>
            <line x1="21" y1="21" x2="16.65" y2="16.65" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          </svg>
          <input type="search"
                 name="q"
                 placeholder="Search"
                 autocomplete="off"
                 role="combobox"
                 aria-expanded="false"
                 aria-haspopup="listbox"
                 aria-autocomplete="list"
                 @input=${this._onSearchInput}
                 @keydown=${this._onKeyDown}>
          <kbd>/</kbd>
        </search>
        <button type="submit">Search</button>
      </form>
      <div role="listbox" aria-label="Search results"></div>
    `;
  }
}

customElements.define('alloy-search', AlloySearch);
