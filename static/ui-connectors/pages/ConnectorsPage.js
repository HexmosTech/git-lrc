const { html, useEffect, useState } = window.preact;

function formatTimestamp(value) {
  if (!value) return '';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return String(value);
  return parsed.toLocaleString();
}

function parseTimestamp(value) {
  if (!value) return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed;
}

function formatRelativeTime(value) {
  const dateValue = parseTimestamp(value);
  if (!dateValue) return '—';

  const deltaMs = Date.now() - dateValue.getTime();
  const absMs = Math.abs(deltaMs);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  const week = 7 * day;
  const month = 30 * day;
  const year = 365 * day;

  let amount = 0;
  let unit = 'minute';

  if (absMs >= year) {
    amount = Math.round(deltaMs / year);
    unit = 'year';
  } else if (absMs >= month) {
    amount = Math.round(deltaMs / month);
    unit = 'month';
  } else if (absMs >= week) {
    amount = Math.round(deltaMs / week);
    unit = 'week';
  } else if (absMs >= day) {
    amount = Math.round(deltaMs / day);
    unit = 'day';
  } else if (absMs >= hour) {
    amount = Math.round(deltaMs / hour);
    unit = 'hour';
  } else {
    amount = Math.round(deltaMs / minute);
    unit = 'minute';
  }

  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
  return formatter.format(-amount, unit);
}

export function ConnectorsPage({
  connectors,
  loading,
  orderedConnectors,
  hasOrderChanges,
  onRefresh,
  onSaveOrder,
  onMove,
  onDragReorder,
  onEdit,
  onDelete,
  onAdd,
}) {
  const [draggingID, setDraggingID] = useState('');
  const [dragOverID, setDragOverID] = useState('');
  const [pulseItemID, setPulseItemID] = useState('');
  const [pulseSave, setPulseSave] = useState(false);

  useEffect(() => {
    if (!pulseItemID) {
      return;
    }
    const timer = window.setTimeout(() => setPulseItemID(''), 900);
    return () => window.clearTimeout(timer);
  }, [pulseItemID]);

  useEffect(() => {
    if (!pulseSave) {
      return;
    }
    const timer = window.setTimeout(() => setPulseSave(false), 900);
    return () => window.clearTimeout(timer);
  }, [pulseSave]);

  function handleDragStart(event, connectorID) {
    const id = String(connectorID);
    setDraggingID(id);
    setDragOverID('');
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', id);
    }
  }

  function handleDragOver(event, connectorID) {
    event.preventDefault();
    if (draggingID && draggingID !== String(connectorID)) {
      setDragOverID(String(connectorID));
    }
  }

  function handleDrop(event, connectorID) {
    event.preventDefault();
    const sourceID = event.dataTransfer?.getData('text/plain') || draggingID;
    const targetID = String(connectorID);

    if (sourceID && targetID && sourceID !== targetID) {
      setPulseItemID(sourceID);
      setPulseSave(true);
    }

    onDragReorder(sourceID, targetID);
    setDraggingID('');
    setDragOverID('');
  }

  function handleDragEnd() {
    setDraggingID('');
    setDragOverID('');
  }

  return html`
    <div class="single">
      <section class="card">
        <h2>AI Connectors (${connectors.length})</h2>
        <div class="connectors-content">
          <div class="toolbar connectors-toolbar">
            <div class="btn-group btn-group-compact">
              <button class="secondary" onClick=${onRefresh} disabled=${loading}>
                <span class="btn-icon" aria-hidden="true">↻</span>${loading ? 'Refreshing...' : 'Refresh'}
              </button>
              <button class=${`${hasOrderChanges ? '' : 'secondary'} ${pulseSave ? 'pulse-attention' : ''}`.trim()} onClick=${onSaveOrder} disabled=${orderedConnectors.length < 2 || !hasOrderChanges}>
                <span class="btn-icon" aria-hidden="true">⇅</span>Save Priority
              </button>
            </div>
            <div class="btn-group">
              <button onClick=${onAdd}>
                <span class="btn-icon" aria-hidden="true">＋</span>Add Connector
              </button>
            </div>
          </div>

          <div class="list">
            ${orderedConnectors.length === 0
              ? html`<div class="page-empty">No connectors found.</div>`
              : orderedConnectors.map((connector, index) => html`
                  <div
                    class=${`item ${draggingID === String(connector.id) ? 'dragging' : ''} ${dragOverID === String(connector.id) ? 'drag-over' : ''} ${pulseItemID === String(connector.id) ? 'pulse-attention' : ''}`}
                    onDragOver=${(event) => handleDragOver(event, connector.id)}
                    onDrop=${(event) => handleDrop(event, connector.id)}
                  >
                    <div class="connector-row">
                      <div class="connector-main">
                        <span class="item-title">${connector.connector_name || 'Connector'}</span>
                        <span class="badge badge-id">#${connector.id}</span>
                        <span class="badge">${connector.provider_name}</span>
                      </div>
                      <div class="row connector-actions">
                        <button
                          class="secondary icon-only drag-handle"
                          title="Drag to reorder"
                          draggable="true"
                          onDragStart=${(event) => handleDragStart(event, connector.id)}
                          onDragEnd=${handleDragEnd}
                        >
                          <span class="btn-icon" aria-hidden="true">⋮⋮</span>
                        </button>
                        <button class="secondary icon-only" title="Move up" disabled=${index === 0} onClick=${() => onMove(String(connector.id), 'up')}>
                          <span class="btn-icon" aria-hidden="true">↑</span>
                        </button>
                        <button class="secondary icon-only" title="Move down" disabled=${index === orderedConnectors.length - 1} onClick=${() => onMove(String(connector.id), 'down')}>
                          <span class="btn-icon" aria-hidden="true">↓</span>
                        </button>
                        <button class="secondary" onClick=${() => onEdit(connector)}>
                          <span class="btn-icon" aria-hidden="true">✎</span>Edit
                        </button>
                        <button class="tertiary-danger" onClick=${() => onDelete(connector.id)}>
                          <span class="btn-icon" aria-hidden="true">🗑</span>Delete
                        </button>
                      </div>
                    </div>
                    <div class="connector-foot">
                      <div class="muted">Priority #${index + 1}${connector.selected_model ? ` · model: ${connector.selected_model}` : ''}</div>
                      <div
                        class="muted muted-meta muted-meta-right"
                        title=${(() => {
                          const created = formatTimestamp(connector.created_at || connector.createdAt);
                          const updated = formatTimestamp(connector.updated_at || connector.updatedAt);
                          if (created && updated) {
                            return `Added: ${created}\nUpdated: ${updated}`;
                          }
                          if (updated) {
                            return `Updated: ${updated}`;
                          }
                          if (created) {
                            return `Added: ${created}`;
                          }
                          return 'Timestamp unavailable';
                        })()}
                      >
                        ${formatRelativeTime(connector.created_at || connector.createdAt || connector.updated_at || connector.updatedAt)}
                      </div>
                    </div>
                  </div>
                `)}
          </div>
        </div>
      </section>
    </div>
  `;
}
