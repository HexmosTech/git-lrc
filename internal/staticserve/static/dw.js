const { h, render } = window.preact;
const { useState, useEffect, useMemo, useRef, useCallback } = window.preactHooks;
const html = window.htm.bind(h);

async function api(path, opts) {
    const res = await fetch(path, opts);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
        data.status = res.status;
        throw data;
    }
    return data;
}

function refDropdownOptions(refs) {
    const groups = [];
    if (refs.branches.length) groups.push(['Branches', refs.branches]);
    if (refs.tags.length) groups.push(['Tags', refs.tags]);
    if (refs.commits.length) groups.push(['Commits', refs.commits.map((c) => c.hash)]);
    return groups;
}

function App() {
    const [refs, setRefs] = useState({ branches: [], tags: [], commits: [], current: '', cached: [] });
    const [selectedRef, setSelectedRef] = useState('');
    const [wiki, setWiki] = useState(null);
    const [selectedPageId, setSelectedPageId] = useState('');
    const [status, setStatus] = useState({ status: 'idle', done: 0, total: 0, page_id: '', error: '', cached: false });
    const [error, setError] = useState('');
    const [loadingWiki, setLoadingWiki] = useState(false);
    const pollRef = useRef(null);

    const loadRefs = useCallback(async () => {
        try {
            const data = await api('/api/dw/refs');
            setRefs(data);
            setSelectedRef((prev) => prev || data.current);
        } catch (e) {
            setError(e.message || String(e));
        }
    }, []);

    useEffect(() => { loadRefs(); }, [loadRefs]);

    const loadWiki = useCallback(async (ref) => {
        if (!ref) return;
        setLoadingWiki(true);
        setWiki(null);
        setSelectedPageId('');
        try {
            const data = await api('/api/dw/wiki?ref=' + encodeURIComponent(ref));
            setWiki(data);
            if (data.wiki_structure && data.wiki_structure.pages && data.wiki_structure.pages.length) {
                setSelectedPageId(data.wiki_structure.pages[0].id);
            }
        } catch (e) {
            if (e && e.status === 404) {
                setWiki(null);
            } else {
                setError(e.message || String(e));
            }
        } finally {
            setLoadingWiki(false);
        }
    }, []);

    useEffect(() => {
        if (selectedRef) loadWiki(selectedRef);
    }, [selectedRef, loadWiki]);

    const stopPolling = useCallback(() => {
        if (pollRef.current) { clearTimeout(pollRef.current); pollRef.current = null; }
    }, []);

    const pollStatus = useCallback(async (ref) => {
        try {
            const s = await api('/api/dw/status?ref=' + encodeURIComponent(ref));
            setStatus(s);
            if (s.status === 'completed' || s.status === 'failed') {
                stopPolling();
                if (s.status === 'completed') await loadWiki(ref);
            } else {
                pollRef.current = setTimeout(() => pollStatus(ref), 800);
            }
        } catch (e) {
            stopPolling();
            setError(e.message || String(e));
        }
    }, [stopPolling, loadWiki]);

    const generate = useCallback(async () => {
        if (!selectedRef) return;
        setError('');
        try {
            await api('/api/dw/generate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ref: selectedRef }),
            });
            setStatus({ status: 'pending', done: 0, total: 0, page_id: '', error: '', cached: false });
            pollStatus(selectedRef);
        } catch (e) {
            setError(e.message || String(e));
        }
    }, [selectedRef, pollStatus]);

    useEffect(() => stopPolling, [stopPolling]);

    const generating = status.status === 'pending' || status.status === 'determining_structure' || status.status === 'generating';

    const structure = wiki ? wiki.wiki_structure : null;
    const page = structure && selectedPageId
        ? structure.pages.find((p) => p.id === selectedPageId)
        : null;

    const renderedContent = useMemo(() => {
        if (!page || !page.content) return '';
        return marked.parse(page.content, { mangle: false, headerIds: false });
    }, [page]);

    const cachedSet = new Set(refs.cached);
    const refOptions = refDropdownOptions(refs);

    const treeNodes = [];
    if (structure) {
        if (structure.sections && structure.sections.length) {
            const byId = Object.fromEntries(structure.pages.map((p) => [p.id, p]));
            const renderSection = (sec) => {
                treeNodes.push(html`<div class="dw-section-title">${sec.title || sec.id}</div>`);
                (sec.pages || []).forEach((pid) => {
                    const p = byId[pid];
                    if (p) treeNodes.push(pageNode(p));
                });
                (sec.subsections || []).forEach((sid) => {
                    const sub = structure.sections.find((s) => s.id === sid);
                    if (sub) renderSection(sub);
                });
            };
            (structure.rootSections && structure.rootSections.length ? structure.rootSections : structure.sections.map((s) => s.id))
                .forEach((sid) => {
                    const sec = structure.sections.find((s) => s.id === sid);
                    if (sec) renderSection(sec);
                });
        } else {
            structure.pages.forEach((p) => treeNodes.push(pageNode(p)));
        }
    }

    function pageNode(p) {
        const active = selectedPageId === p.id;
        return html`
            <div class=${'dw-page' + (active ? ' active' : '')} onClick=${() => setSelectedPageId(p.id)}>
                ${p.title || p.id}
            </div>`;
    }

    const progressPct = status.total ? Math.round((status.done / status.total) * 100) : 0;

    return html`
        <div class="dw-header">
            <h1>DeepWiki</h1>
            <span class="dw-ref-label">ref:</span>
            <select value=${selectedRef} onChange=${(e) => setSelectedRef(e.target.value)}>
                ${refOptions.map(([label, values]) => html`
                    <optgroup label=${label}>
                        ${values.map((v) => html`<option value=${v}>${v}</option>`)}
                    </optgroup>`)}
            </select>
            <span class="dw-badge">${cachedSet.has(selectedRef) ? 'generated' : 'not generated'}</span>
            <div class="dw-actions">
                <button class="dw-btn primary" onClick=${generate} disabled=${generating}>
                    ${generating ? 'Generating…' : (cachedSet.has(selectedRef) ? 'Regenerate' : 'Generate')}
                </button>
            </div>
        </div>

        ${generating ? html`
            <div class="dw-progress"><div style=${'width:' + progressPct + '%'}></div></div>
            <div class="dw-status-line" style=${{ padding: '4px 16px', fontSize: '12px', color: 'var(--dw-muted)' }}>
                ${status.status} ${status.total ? '(' + status.done + '/' + status.total + ')' : ''} ${status.page_id ? '· ' + status.page_id : ''}
            </div>
        ` : ''}

        ${error ? html`<div class="dw-error">${error}</div>` : ''}

        <div class="dw-body">
            <div class="dw-refs">
                <div class="dw-group-title">Branches</div>
                ${refs.branches.map((r) => html`
                    <div class=${'dw-ref-item' + (selectedRef === r ? ' active' : '')} onClick=${() => setSelectedRef(r)}>
                        <span class=${'dot ' + (cachedSet.has(r) ? 'cached' : 'uncached')}></span>${r}
                    </div>`)}
                <div class="dw-group-title">Tags</div>
                ${refs.tags.map((r) => html`
                    <div class=${'dw-ref-item' + (selectedRef === r ? ' active' : '')} onClick=${() => setSelectedRef(r)}>
                        <span class=${'dot ' + (cachedSet.has(r) ? 'cached' : 'uncached')}></span>${r}
                    </div>`)}
                <div class="dw-group-title">Commits</div>
                ${refs.commits.map((c) => html`
                    <div class=${'dw-ref-item' + (selectedRef === c.hash ? ' active' : '')} onClick=${() => setSelectedRef(c.hash)} title=${c.subject}>
                        <span class=${'dot ' + (cachedSet.has(c.hash) ? 'cached' : 'uncached')}></span>${c.hash.slice(0, 7)}
                    </div>`)}
            </div>

            <div class="dw-tree">
                ${structure ? treeNodes : html`<div class="dw-empty">No structure</div>`}
            </div>

            <div class="dw-content">
                ${loadingWiki ? html`<div class="dw-empty">Loading…</div>` : ''}
                ${!loadingWiki && !wiki ? html`
                    <div class="dw-empty">
                        <h2>No documentation for "${selectedRef}"</h2>
                        <p>Generate the wiki for this ref to see the docs.</p>
                        <button class="dw-btn primary" onClick=${generate} disabled=${generating}>Generate</button>
                    </div>` : ''}
                ${!loadingWiki && wiki && page ? html`
                    <div class="dw-markdown" dangerouslySetInnerHTML=${{ __html: renderedContent }}></div>
                ` : ''}
            </div>
        </div>
    `;
}

render(html`<${App} />`, document.getElementById('app'));
