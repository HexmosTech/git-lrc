const { html } = window.preact;

export function ConnectorFormPage({
  title,
  form,
  providers,
  selectedProvider,
  modelOptions,
  fetchingModels,
  modelsFetched,
  saving,
  saveDisabled,
  status,
  error,
  onProviderChange,
  onFieldChange,
  onFetchOllamaModels,
  onSave,
  onCancel,
}) {
  const isOllama = form.provider_name === 'ollama';
  const showBaseURL = Boolean(selectedProvider.requiresBaseURL);
  const connectorName = (form.connector_name || '').trim();
  const apiKey = (form.api_key || '').trim();
  const baseURL = (form.base_url || '').trim();

  let hasValidBaseURL = true;
  if (showBaseURL) {
    try {
      const parsedURL = new URL(baseURL);
      hasValidBaseURL = (parsedURL.protocol === 'http:' || parsedURL.protocol === 'https:') && Boolean(parsedURL.host);
    } catch {
      hasValidBaseURL = false;
    }
  }

  const hasClientValidationError =
    !connectorName ||
    (!isOllama && !apiKey) ||
    (showBaseURL && (!baseURL || !hasValidBaseURL));

  const fetchModelsDisabled = fetchingModels || !baseURL || !hasValidBaseURL;
  const effectiveSaveDisabled = saving || saveDisabled || hasClientValidationError;

  return html`
    <div class="single">
      <section class="card">
        <h2>${title}</h2>
        <div class="form-content">
          <label>Provider</label>
          <select value=${form.provider_name} onChange=${(event) => onProviderChange(event.target.value)}>
            ${providers.map((provider) => html`<option value=${provider.id}>${provider.name}</option>`)}
          </select>

          <label>Connector Name</label>
          <input
            value=${form.connector_name}
            required
            onInput=${(event) => onFieldChange('connector_name', event.target.value)}
          />

          <label>${isOllama ? 'JWT Token (optional)' : 'API Key'}</label>
          <input
            type="password"
            value=${form.api_key}
            required=${!isOllama}
            autoComplete="new-password"
            spellcheck="false"
            placeholder=${selectedProvider.apiKeyPlaceholder || ''}
            onInput=${(event) => onFieldChange('api_key', event.target.value)}
          />

          ${showBaseURL
            ? html`
                <label>Base URL (required)</label>
                <input
                  type="url"
                  required
                  placeholder=${selectedProvider.baseURLPlaceholder || 'http://localhost:11434/ollama/api'}
                  value=${form.base_url}
                  onInput=${(event) => onFieldChange('base_url', event.target.value)}
                />
              `
            : ''}

          ${isOllama
            ? html`
                <label>Available Models</label>
                <div class="row">
                  <button class="secondary" disabled=${fetchModelsDisabled} onClick=${onFetchOllamaModels}>
                    <span class="btn-icon" aria-hidden="true">◎</span>${fetchingModels ? 'Fetching...' : 'Fetch Models'}
                  </button>
                </div>

                ${baseURL && !hasValidBaseURL
                  ? html`<div class="status err">Enter a valid Base URL (http:// or https://) before fetching models.</div>`
                  : ''}

                ${!modelsFetched && form.selected_model
                  ? html`
                      <div class="status ok">
                        Currently selected model: ${form.selected_model}. Fetch models to change selection.
                      </div>
                    `
                  : ''}

                ${!modelsFetched
                  ? html`<div class="status">Click "Fetch Models" to load available models from your Ollama instance.</div>`
                  : ''}

                ${modelOptions.length > 0
                  ? html`
                      <select
                        value=${form.selected_model}
                        onChange=${(event) => onFieldChange('selected_model', event.target.value)}
                      >
                        <option value="">Select a model</option>
                        ${modelOptions.map((model) => html`<option value=${model}>${model}</option>`)}
                      </select>
                    `
                  : ''}

                ${modelsFetched && modelOptions.length === 0
                  ? html`<div class="status err">No models found. Pull models in Ollama first.</div>`
                  : ''}
              `
            : html`
                <label>Model</label>
                ${modelOptions.length > 0
                  ? html`
                      <select
                        value=${form.selected_model}
                        onChange=${(event) => onFieldChange('selected_model', event.target.value)}
                      >
                        ${modelOptions.map((model) => html`<option value=${model}>${model}</option>`)}
                      </select>
                    `
                  : html`<input value=${form.selected_model} onInput=${(event) => onFieldChange('selected_model', event.target.value)} />`}
              `}

          <div class="row">
            <button disabled=${effectiveSaveDisabled} onClick=${onSave}>
              <span class="btn-icon" aria-hidden="true">💾</span>${saving ? 'Saving...' : form.id ? 'Update' : 'Create'}
            </button>
            <button class="secondary" onClick=${onCancel}>
              <span class="btn-icon" aria-hidden="true">↩</span>Cancel
            </button>
          </div>

          ${status ? html`<div class=${`status ${error ? 'err' : 'ok'}`}>${status}</div>` : ''}
        </div>
      </section>
    </div>
  `;
}
