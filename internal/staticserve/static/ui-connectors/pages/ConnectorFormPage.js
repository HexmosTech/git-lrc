const { html } = window.preact;

export function ConnectorFormPage({
  title,
  form,
  providers,
  selectedProvider,
  modelOptions,
  filteredModels = [],
  searchQuery = '',
  setSearchQuery,
  isOpen = false,
  setIsOpen,
  dropdownRef,
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
  onGenerateName,
  onCancel,
  connectorNamePlaceholder,
  apiDefaultModel = '',
}) {
  const isOllama = form.provider_name === 'ollama';
  const isGeminiEnterprise = form.provider_name === 'gemini-enterprise';
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

  const handleFileChange = (e) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (file.size > 1024 * 1024) {
      alert('File is too large. Please upload a valid service account JSON under 1MB.');
      return;
    }

    const reader = new FileReader();
    reader.onload = (event) => {
      const content = event.target.result;
      onFieldChange('api_key', content);
      
      try {
        const parsed = JSON.parse(content);
        if (parsed.project_id) {
          onFieldChange('gcp_project_id', parsed.project_id);
        }
      } catch (err) {
        console.error('Failed to parse Service Account JSON:', err);
      }
    };
    reader.readAsText(file);
  };

  const hasClientValidationError =
    !connectorName ||
    (isGeminiEnterprise && (!apiKey || !form.gcp_project_id || !form.gcp_location)) ||
    (!isOllama && !isGeminiEnterprise && !apiKey) ||
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
          <div class="connector-name-row">
            <input
              value=${form.connector_name}
              required
              placeholder=${connectorNamePlaceholder || 'Enter a connector name'}
              onInput=${(event) => onFieldChange('connector_name', event.target.value)}
            />
            <button class="secondary subtle-action" onClick=${onGenerateName} title="Generate a smart connector name">
              Regenerate
            </button>
          </div>

          ${isGeminiEnterprise
            ? html`
                <label>Service Account JSON</label>
                <div class="file-upload-container" style="display: flex; align-items: center; justify-content: space-between; border: 1px dashed var(--border-medium, #454545); background: var(--bg-tertiary, #2d2d30); border-radius: 6px; padding: 16px; margin-bottom: 15px;">
                  <div style="display: flex; align-items: center; gap: 12px;">
                    <div style="padding: 8px; background: rgba(0, 122, 204, 0.1); border-radius: 6px; color: var(--text-link, #007acc); display: flex; align-items: center; justify-content: center;">
                      <svg style="width: 24px; height: 24px;" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                      </svg>
                    </div>
                    <div>
                      <h4 style="margin: 0; font-size: 14px; font-weight: 500; color: var(--text-primary, #cccccc);">
                        ${form.api_key ? "Service Account JSON Loaded" : "Upload Credentials File"}
                      </h4>
                      <p style="margin: 4px 0 0 0; font-size: 12px; color: var(--text-muted, #858585);">
                        ${form.api_key 
                          ? `Valid Google Cloud service account JSON configured (${(form.api_key.length / 1024).toFixed(2)} KB)`
                          : "Upload the Google Cloud IAM Service Account JSON keyfile"
                        }
                      </p>
                    </div>
                  </div>
                  <div>
                    <label class="cursor-pointer" style="cursor: pointer;">
                      <span style="display: inline-flex; align-items: center; padding: 6px 12px; border: 1px solid var(--border-medium, #454545); background: transparent; font-size: 13px; font-weight: 500; border-radius: 4px; color: var(--text-link, #007acc); transition: all 0.2s;">
                        ${form.api_key ? "Replace File" : "Choose File"}
                      </span>
                      <input
                        type="file"
                        accept=".json"
                        onChange=${handleFileChange}
                        style="display: none;"
                      />
                    </label>
                  </div>
                </div>
                <p style="margin: -5px 0 15px 0; font-size: 12px; color: var(--text-muted, #858585); line-height: 1.5;">
                  Follow this guide to <a href="https://developers.google.com/workspace/guides/create-credentials#service-account" target="_blank" rel="noopener noreferrer" style="color: var(--text-link, #007acc); text-decoration: underline;">create a service account JSON file</a> and assign the <strong>Agent Platform user</strong> role.
                </p>

                <label>GCP Project ID</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. my-gcp-project-id"
                  value=${form.gcp_project_id || ''}
                  onInput=${(event) => onFieldChange('gcp_project_id', event.target.value)}
                />

                <label>GCP Location (Region)</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. us-central1, europe-west9"
                  value=${form.gcp_location || ''}
                  onInput=${(event) => onFieldChange('gcp_location', event.target.value)}
                />
              `
            : html`
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
              `
          }

          ${showBaseURL
            ? (() => {
                const presets = selectedProvider.baseURLPresets || [];
                const CUSTOM = '__custom__';
                const effectivePreset = presets.find((p) => p.value === form.base_url) ? form.base_url : CUSTOM;
                if (presets.length > 0) {
                  return html`
                    <label>Base URL (required)</label>
                    <select
                      value=${effectivePreset}
                      onChange=${(e) => {
                        const v = e.target.value;
                        onFieldChange('base_url', v === CUSTOM ? '' : v);
                      }}
                    >
                      ${presets.map((p) => html`<option value=${p.value}>${p.label}</option>`)}
                      <option value=${CUSTOM}>Custom URL...</option>
                    </select>
                    ${effectivePreset === CUSTOM
                      ? html`
                          <input
                            type="url"
                            required
                            placeholder=${selectedProvider.baseURLPlaceholder || 'https://'}
                            value=${form.base_url}
                            onInput=${(event) => onFieldChange('base_url', event.target.value)}
                          />
                        `
                      : ''}
                  `;
                }
                return html`
                  <label>Base URL (required)</label>
                  <input
                    type="url"
                    required
                    placeholder=${selectedProvider.baseURLPlaceholder || 'http://localhost:11434/ollama/api'}
                    value=${form.base_url}
                    onInput=${(event) => onFieldChange('base_url', event.target.value)}
                  />
                `;
              })()
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
                ${fetchingModels
                  ? html`
                      <select disabled class="loading-select">
                        <option>Loading models...</option>
                      </select>
                    `
                  : modelOptions.length > 0
                    ? html`
                        <div ref=${dropdownRef} class="custom-select-wrapper" style="position: relative; width: 100%; z-index: ${isOpen ? '100' : '1'}; margin-bottom: 10px;">
                          <!-- Trigger Button styled exactly like a native select -->
                          <button
                            type="button"
                            class="custom-select-trigger"
                            style="width: 100%; background: var(--bg-tertiary, #2d2d30); color: var(--text-secondary, #d4d4d4); border: 1px solid var(--border-medium, #454545); border-radius: 4px; padding: 10px; text-align: left; display: flex; justify-content: space-between; align-items: center; cursor: pointer; outline: none;"
                            onClick=${() => setIsOpen(!isOpen)}
                          >
                            <span class="truncate" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 90%;">
                              ${form.selected_model
                                ? `${form.selected_model}${form.selected_model === apiDefaultModel ? ' (Recommended)' : ''}`
                                : 'Select a model'}
                            </span>
                            <span style="color: var(--text-muted, #858585); font-size: 10px; margin-left: 8px;">${isOpen ? '▲' : '▼'}</span>
                          </button>

                          ${isOpen
                            ? html`
                                <div
                                  class="custom-select-options-container"
                                  style="position: absolute; top: 100%; left: 0; right: 0; margin-top: 4px; background: var(--bg-tertiary, #2d2d30); border: 1px solid var(--border-medium, #454545); border-radius: 4px; box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.5); max-height: 280px; overflow-y: auto; z-index: 9999;"
                                >
                                  <!-- Search box pinned to the top, styled like standard inputs -->
                                  <div style="position: sticky; top: 0; padding: 8px; background: var(--bg-tertiary, #2d2d30); border-bottom: 1px solid var(--border-subtle, #3c3c3c); z-index: 10;">
                                    <input
                                      type="text"
                                      placeholder="Search model name..."
                                      value=${searchQuery}
                                      onInput=${(e) => setSearchQuery(e.target.value)}
                                      onClick=${(e) => e.stopPropagation()}
                                      style="width: 100%; background: var(--bg-primary, #1e1e1e); color: var(--text-primary, #cccccc); border: 1px solid var(--border-medium, #454545); border-radius: 4px; padding: 8px 10px; font-size: 13px; box-sizing: border-box; margin-bottom: 0;"
                                      autofocus
                                    />
                                  </div>

                                  <!-- Options list -->
                                  <div style="padding: 4px 0;">
                                    ${filteredModels.map(
                                      (model) => html`
                                        <button
                                          type="button"
                                          class="custom-select-option"
                                          style="width: 100%; text-align: left; padding: 8px 12px; font-size: 13px; background: ${form.selected_model === model ? 'var(--bg-active, #37373d)' : 'transparent'}; color: ${form.selected_model === model ? '#fff' : 'var(--text-secondary, #d4d4d4)'}; border: none; cursor: pointer; display: flex; justify-content: space-between; align-items: center;"
                                          onClick=${() => {
                                            onFieldChange('selected_model', model);
                                            setIsOpen(false);
                                            setSearchQuery('');
                                          }}
                                          onMouseEnter=${(e) => {
                                            e.currentTarget.style.background = 'var(--bg-hover, #2a2d2e)';
                                            e.currentTarget.style.color = 'var(--text-primary, #cccccc)';
                                          }}
                                          onMouseLeave=${(e) => {
                                            e.currentTarget.style.background = form.selected_model === model ? 'var(--bg-active, #37373d)' : 'transparent';
                                            e.currentTarget.style.color = form.selected_model === model ? '#fff' : 'var(--text-secondary, #d4d4d4)';
                                          }}
                                        >
                                          <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 75%;">${model}</span>
                                          ${model === apiDefaultModel
                                            ? html`<span style="font-size: 10px; background: var(--bg-active, #37373d); color: var(--text-muted, #858585); padding: 2px 6px; border-radius: 4px; margin-left: 8px; white-space: nowrap;">Recommended</span>`
                                            : ''}
                                        </button>
                                      `
                                    )}

                                    ${filteredModels.length === 0
                                      ? html`<div style="padding: 10px 12px; font-size: 13px; color: var(--text-muted, #858585); font-style: italic;">No matching models found</div>`
                                      : ''}
                                  </div>
                                </div>
                              `
                            : ''}
                        </div>
                      `
                    : (() => {
                        const selectedBasePreset = (selectedProvider.baseURLPresets || []).find((p) => p.value === form.base_url);
                        const mPresets = selectedBasePreset?.models || selectedProvider.modelPresets || [];
                        const CUSTOM_MODEL = '__custom_model__';
                        if (mPresets.length > 0) {
                          const effectiveModel = mPresets.includes(form.selected_model) ? form.selected_model : CUSTOM_MODEL;
                          return html`
                            <select
                              value=${effectiveModel}
                              onChange=${(e) => {
                                const v = e.target.value;
                                onFieldChange('selected_model', v === CUSTOM_MODEL ? '' : v);
                              }}
                            >
                              ${mPresets.map((m) => html`<option value=${m}>${m}</option>`)}
                              <option value=${CUSTOM_MODEL}>+ Custom</option>
                            </select>
                            ${effectiveModel === CUSTOM_MODEL
                              ? html`
                                  <input
                                    value=${form.selected_model}
                                    placeholder="Enter model ID (e.g., claude-haiku-4-5-20251001)"
                                    onInput=${(event) => onFieldChange('selected_model', event.target.value)}
                                  />
                                `
                              : ''}
                          `;
                        }
                        return html`
                          <input
                            value=${form.selected_model}
                            placeholder="Enter model ID (e.g., gpt-4o)"
                            onInput=${(event) => onFieldChange('selected_model', event.target.value)}
                          />
                        `;
                      })()}
              `
            }

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
