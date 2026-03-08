import { api } from '/static/ui-connectors/api.js';
import { providers, defaultForm } from '/static/ui-connectors/providers.js';
import { parseRoute, routePath, navigate } from '/static/ui-connectors/router.js';
import { HeaderNav } from '/static/ui-connectors/components/HeaderNav.js';
import { Breadcrumbs } from '/static/ui-connectors/components/Breadcrumbs.js';
import { HomePage } from '/static/ui-connectors/pages/HomePage.js';
import { ConnectorsPage } from '/static/ui-connectors/pages/ConnectorsPage.js';
import { ConnectorFormPage } from '/static/ui-connectors/pages/ConnectorFormPage.js';

const { render, useEffect, useMemo, useRef, useState, html } = window.preact;

function App() {
  const suppressHashChangeRef = useRef(false);
  const [route, setRoute] = useState(parseRoute(window.location.hash));
  const [connectors, setConnectors] = useState([]);
  const [draftOrder, setDraftOrder] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [status, setStatus] = useState('');
  const [saving, setSaving] = useState(false);
  const [fetchingModels, setFetchingModels] = useState(false);
  const [modelsFetched, setModelsFetched] = useState(false);
  const [form, setForm] = useState(defaultForm());
  const [ollamaModels, setOllamaModels] = useState([]);

  const selectedProvider = useMemo(() => {
    return providers.find((provider) => provider.id === form.provider_name) || providers[0];
  }, [form.provider_name]);

  const modelOptions = useMemo(() => {
    if (form.provider_name === 'ollama' && ollamaModels.length > 0) {
      return ollamaModels;
    }
    return selectedProvider.models || [];
  }, [selectedProvider, form.provider_name, ollamaModels]);

  const hasOrderChanges = useMemo(() => {
    const currentOrder = [...connectors]
      .sort((left, right) => (left.display_order || 0) - (right.display_order || 0))
      .map((item) => String(item.id));

    if (currentOrder.length !== draftOrder.length) {
      return true;
    }

    for (let index = 0; index < currentOrder.length; index++) {
      if (currentOrder[index] !== draftOrder[index]) {
        return true;
      }
    }

    return false;
  }, [connectors, draftOrder]);

  const activePath = routePath(route);
  const editingConnector =
    route.name === 'edit'
      ? connectors.find((entry) => String(entry.id) === String(route.connectorID))
      : null;
  const breadcrumbConnectorLabel =
    editingConnector?.connector_name || form.connector_name || (route.connectorID ? `Connector #${route.connectorID}` : 'Edit Connector');

  const pageTitle = useMemo(() => {
    if (route.name === 'connectors') {
      return 'git-lrc Manager | AI Connectors | List';
    }

    if (route.name === 'new') {
      return 'git-lrc Manager | AI Connectors | Add Connector';
    }

    if (route.name === 'edit') {
      const label = breadcrumbConnectorLabel ? ` | ${breadcrumbConnectorLabel}` : '';
      return `git-lrc Manager | AI Connectors | Edit Connector${label}`;
    }

    return 'git-lrc Manager | Home';
  }, [route.name, breadcrumbConnectorLabel]);

  async function loadConnectors() {
    setLoading(true);
    setError('');
    try {
      const data = await api('/api/ui/connectors');
      const sorted = [...data].sort((left, right) => (left.display_order || 0) - (right.display_order || 0));
      setConnectors(sorted);
      setDraftOrder(sorted.map((item) => String(item.id)));
      setStatus(`Loaded ${sorted.length} connector(s)`);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadConnectors();
  }, []);

  useEffect(() => {
    document.title = pageTitle;
  }, [pageTitle]);

  useEffect(() => {
    const onHashChange = () => {
      if (suppressHashChangeRef.current) {
        suppressHashChangeRef.current = false;
        return;
      }

      const nextRoute = parseRoute(window.location.hash);
      const leavingConnectorsWithUnsavedOrder =
        route.name === 'connectors' &&
        hasOrderChanges &&
        nextRoute.name !== 'connectors';

      if (leavingConnectorsWithUnsavedOrder) {
        const shouldLeave = window.confirm('You have unsaved priority changes. Leave without saving?');
        if (!shouldLeave) {
          suppressHashChangeRef.current = true;
          window.location.hash = routePath(route);
          return;
        }

        const savedOrder = [...connectors]
          .sort((left, right) => (left.display_order || 0) - (right.display_order || 0))
          .map((item) => String(item.id));
        setDraftOrder(savedOrder);
      }

      setRoute(nextRoute);
    };

    window.addEventListener('hashchange', onHashChange);
    onHashChange();
    return () => window.removeEventListener('hashchange', onHashChange);
  }, [route, hasOrderChanges]);

  useEffect(() => {
    const onBeforeUnload = (event) => {
      const hasUnsavedOrderChanges = route.name === 'connectors' && hasOrderChanges;
      if (!hasUnsavedOrderChanges) {
        return;
      }

      event.preventDefault();
      event.returnValue = '';
    };

    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [route.name, hasOrderChanges]);

  useEffect(() => {
    if (route.name === 'new') {
      resetFormState();
      setError('');
      setStatus('');
      return;
    }

    if (route.name === 'edit') {
      const connector = connectors.find((entry) => String(entry.id) === String(route.connectorID));
      if (connector) {
        setOllamaModels([]);
        setModelsFetched(false);
        setFetchingModels(false);
        setForm({
          id: String(connector.id),
          provider_name: connector.provider_name || 'gemini',
          connector_name: connector.connector_name || 'Connector',
          api_key: connector.api_key || '',
          base_url: connector.base_url || '',
          selected_model: connector.selected_model || '',
        });
        setStatus(`Editing connector #${connector.id}`);
      }
    }
  }, [route.name, route.connectorID, connectors]);

  function resetFormState() {
    setForm(defaultForm());
    setOllamaModels([]);
    setModelsFetched(false);
    setFetchingModels(false);
  }

  function setProvider(providerId) {
    const provider = providers.find((entry) => entry.id === providerId) || providers[0];
    setForm((prev) => ({
      ...prev,
      provider_name: provider.id,
      selected_model: provider.defaultModel,
      base_url: provider.requiresBaseURL ? prev.base_url : '',
    }));
    if (provider.id !== 'ollama') {
      setOllamaModels([]);
      setModelsFetched(false);
    }
  }

  function setFormField(field, value) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function editConnector(connector) {
    setForm({
      id: String(connector.id),
      provider_name: connector.provider_name || 'gemini',
      connector_name: connector.connector_name || 'Connector',
      api_key: connector.api_key || '',
      base_url: connector.base_url || '',
      selected_model: connector.selected_model || '',
    });
    setStatus(`Editing connector #${connector.id}`);
    navigate(`/connectors/edit/${connector.id}`);
  }

  async function fetchOllamaModels() {
    setFetchingModels(true);
    setError('');
    setStatus('');
    if (!form.base_url) {
      setError('Base URL is required for Ollama model discovery');
      setFetchingModels(false);
      return;
    }
    try {
      const response = await api('/api/ui/connectors/ollama/models', {
        method: 'POST',
        body: JSON.stringify({
          base_url: form.base_url,
          jwt_token: form.api_key || undefined,
        }),
      });
      const models = response.models || [];
      setOllamaModels(models);
      setModelsFetched(true);
      setForm((prev) => {
        if (prev.selected_model && models.includes(prev.selected_model)) {
          return prev;
        }
        return { ...prev, selected_model: '' };
      });
      setStatus(`Fetched ${models.length} Ollama model(s)`);
    } catch (err) {
      setModelsFetched(false);
      setError(err.message || String(err));
    } finally {
      setFetchingModels(false);
    }
  }

  async function saveConnector() {
    const provider = providers.find((entry) => entry.id === form.provider_name) || providers[0];
    const connectorName = (form.connector_name || '').trim();
    const apiKey = (form.api_key || '').trim();
    const baseURL = (form.base_url || '').trim();
    const selectedModel = (form.selected_model || '').trim();
    let hasValidBaseURL = true;
    if (provider.requiresBaseURL) {
      try {
        const parsedURL = new URL(baseURL);
        hasValidBaseURL =
          (parsedURL.protocol === 'http:' || parsedURL.protocol === 'https:') &&
          Boolean(parsedURL.host);
      } catch {
        hasValidBaseURL = false;
      }
    }

    if (!connectorName) {
      setError('Connector name is required');
      setStatus('');
      return;
    }

    if (provider.id === 'ollama') {
      if (!baseURL) {
        setError('Base URL is required');
        setStatus('');
        return;
      }

      if (!hasValidBaseURL) {
        setError('Base URL must be a valid http:// or https:// URL');
        setStatus('');
        return;
      }

      if (!selectedModel) {
        setError('Please select a model');
        setStatus('');
        return;
      }
    } else if (!apiKey) {
      setError('API key is required');
      setStatus('');
      return;
    }

    setSaving(true);
    setError('');
    setStatus('');

    try {
      if (provider.id !== 'ollama') {
        setStatus('Validating API key...');
        const validationResponse = await api('/api/ui/connectors/validate-key', {
          method: 'POST',
          body: JSON.stringify({
            provider: provider.id,
            api_key: apiKey,
            model: selectedModel || provider.defaultModel || undefined,
          }),
        });

        if (!validationResponse.valid) {
          setError(validationResponse.message || 'API key validation failed');
          setStatus('');
          return;
        }
      }

      const payload = {
        provider_name: provider.id,
        api_key: apiKey,
        connector_name: connectorName,
        base_url: provider.requiresBaseURL ? baseURL : undefined,
        selected_model: selectedModel || provider.defaultModel || undefined,
        display_order: 0,
      };

      if (form.id) {
        await api(`/api/ui/connectors/${form.id}`, {
          method: 'PUT',
          body: JSON.stringify(payload),
        });
        setStatus('Connector updated');
      } else {
        await api('/api/ui/connectors', {
          method: 'POST',
          body: JSON.stringify(payload),
        });
        setStatus('Connector created');
      }
      resetFormState();
      await loadConnectors();
      navigate('/connectors');
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setSaving(false);
    }
  }

  async function deleteConnector(connectorId) {
    if (!confirm('Delete this connector?')) {
      return;
    }
    setError('');
    setStatus('');
    try {
      await api(`/api/ui/connectors/${connectorId}`, { method: 'DELETE' });
      setStatus(`Connector #${connectorId} deleted`);
      await loadConnectors();
    } catch (err) {
      setError(err.message || String(err));
    }
  }

  function move(id, direction) {
    setDraftOrder((prev) => {
      const index = prev.findIndex((value) => value === id);
      if (index < 0) return prev;
      const target = direction === 'up' ? index - 1 : index + 1;
      if (target < 0 || target >= prev.length) return prev;
      const copy = [...prev];
      [copy[index], copy[target]] = [copy[target], copy[index]];
      return copy;
    });
  }

  function reorderByDrag(sourceID, targetID) {
    if (!sourceID || !targetID || sourceID === targetID) {
      return;
    }

    setDraftOrder((prev) => {
      const sourceIndex = prev.findIndex((value) => value === sourceID);
      const targetIndex = prev.findIndex((value) => value === targetID);
      if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) {
        return prev;
      }

      const copy = [...prev];
      const [sourceValue] = copy.splice(sourceIndex, 1);
      copy.splice(targetIndex, 0, sourceValue);
      return copy;
    });
  }

  async function saveOrder() {
    setError('');
    setStatus('');
    try {
      const updates = draftOrder.map((id, idx) => ({ id, display_order: idx + 1 }));
      await api('/api/ui/connectors/reorder', {
        method: 'PUT',
        body: JSON.stringify(updates),
      });
      setStatus('Priority order saved');
      await loadConnectors();
    } catch (err) {
      setError(err.message || String(err));
    }
  }

  const orderedConnectors = draftOrder
    .map((id) => connectors.find((entry) => String(entry.id) === id))
    .filter(Boolean);

  const saveDisabled = useMemo(() => {
    const provider = providers.find((entry) => entry.id === form.provider_name) || providers[0];
    const connectorName = (form.connector_name || '').trim();
    const apiKey = (form.api_key || '').trim();
    const baseURL = (form.base_url || '').trim();
    const selectedModel = (form.selected_model || '').trim();

    if (!connectorName) {
      return true;
    }

    if (provider.id === 'ollama') {
      return !baseURL || !selectedModel;
    }

    if (!apiKey) {
      return true;
    }

    if (provider.id === 'openrouter' && !selectedModel) {
      return true;
    }

    return false;
  }, [form]);

  function renderPage() {
    if (route.name === 'connectors') {
      return html`<${ConnectorsPage}
        connectors=${connectors}
        loading=${loading}
        orderedConnectors=${orderedConnectors}
        hasOrderChanges=${hasOrderChanges}
        onRefresh=${loadConnectors}
        onSaveOrder=${saveOrder}
        onMove=${move}
        onDragReorder=${reorderByDrag}
        onEdit=${editConnector}
        onDelete=${deleteConnector}
        onAdd=${() => navigate('/connectors/new')}
      />`;
    }

    if (route.name === 'new' || route.name === 'edit') {
      return html`<${ConnectorFormPage}
        title=${form.id ? 'Edit Connector' : 'Add Connector'}
        form=${form}
        providers=${providers}
        selectedProvider=${selectedProvider}
        modelOptions=${modelOptions}
        fetchingModels=${fetchingModels}
        modelsFetched=${modelsFetched}
        saving=${saving}
        saveDisabled=${saveDisabled}
        status=${status}
        error=${error}
        onProviderChange=${setProvider}
        onFieldChange=${setFormField}
        onFetchOllamaModels=${fetchOllamaModels}
        onSave=${saveConnector}
        onCancel=${() => {
          resetFormState();
          navigate('/connectors');
        }}
      />`;
    }

    return html`<${HomePage} />`;
  }

  return html`
    <div class="wrap ui-shell">
      <${HeaderNav} activePath=${activePath} />
      <${Breadcrumbs} route=${route} connectorName=${breadcrumbConnectorLabel} />
      ${error ? html`<div class="err-banner">${error}</div>` : ''}
      ${renderPage()}
    </div>
  `;
}

render(html`<${App} />`, document.getElementById('app'));
