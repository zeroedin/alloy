import { LitElement, html, css } from 'lit';

export class DsCard extends LitElement {
  static properties = {
    title: { type: String }
  };

  render() {
    return html`
      <div class="card">
        <h2>${this.title}</h2>
        <slot></slot>
      </div>
    `;
  }
}
customElements.define('ds-card', DsCard);
