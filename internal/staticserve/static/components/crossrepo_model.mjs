// Pure cross-repo presentation logic, unit-tested under `node --test`.
//
// The local review UI gets cross-repo impacts from LiveReview's match
// endpoint. Older artifacts (and the server worker) may still carry the
// engine's index-cache project id in RelatedRepo, so every display path goes
// through the same resolver: friendly full_name first, then the provider URL
// path, then a decoded id (`repo_cache-...-org__service-repo` -> org/service).
// Nothing here touches the DOM.

import { buildHierarchy } from './callgraph_model.mjs';

function looksLikeEngineProject(name) {
    return /repo_cache-\d+-\d+-/.test(name) || (name.includes('__') && /-repo$/.test(name));
}

function decodeEngineProject(raw) {
    const m = (raw || '').match(/repo_cache-\d+-\d+-(.+?)-repo$/);
    if (m) return m[1].replace(/__/g, '/');
    if ((raw || '').includes('__')) return raw.replace(/__/g, '/');
    return raw || '';
}

function repoNameFromWebURL(raw) {
    if (!raw) return '';
    try {
        const url = new URL(raw);
        const path = url.pathname.replace(/^\/+|\/+$/g, '').split('/-/')[0];
        return path.replace(/\.git$/, '');
    } catch {
        return '';
    }
}

export function relatedRepoDisplayName(ref) {
    const explicit = String((ref && ref.RelatedRepoName) || '').trim();
    if (explicit && !looksLikeEngineProject(explicit)) return explicit;
    const fromURL = repoNameFromWebURL(ref && ref.RelatedRepoWebURL);
    if (fromURL) return fromURL;
    return decodeEngineProject(String(explicit || (ref && ref.RelatedRepo) || '').trim()) || 'Unknown repository';
}

export function relatedRepoNameVariants(ref) {
    const out = new Set();
    [ref && ref.RelatedRepoName, ref && ref.RelatedRepo, relatedRepoDisplayName(ref)].forEach((v) => {
        const trimmed = String(v || '').trim();
        if (trimmed) out.add(trimmed);
    });
    return Array.from(out).sort((a, b) => b.length - a.length);
}

// __route__POST__/v1/orders -> /v1/orders
export function crossRepoRouteLabel(callSite) {
    return (callSite || '').replace(/^__route__[A-Z]+__/, '');
}

function branchKey(im) {
    if (im && im.RelatedRepoID) return `id:${im.RelatedRepoID}`;
    return `name:${relatedRepoDisplayName(im)}`;
}

export function groupRelatedRepos(impacts) {
    const groups = new Map();
    (impacts || []).forEach((im) => {
        const key = branchKey(im);
        let group = groups.get(key);
        if (!group) {
            group = {
                key,
                id: im.RelatedRepoID || null,
                name: relatedRepoDisplayName(im),
                webURL: im.RelatedRepoWebURL || '',
                callSites: [],
                matchKinds: [],
                edgeTypes: [],
            };
            groups.set(key, group);
        }
        if (im.CallSite && !group.callSites.includes(im.CallSite)) group.callSites.push(im.CallSite);
        if (im.MatchKind && !group.matchKinds.includes(im.MatchKind)) group.matchKinds.push(im.MatchKind);
        if (im.EdgeType && !group.edgeTypes.includes(im.EdgeType)) group.edgeTypes.push(im.EdgeType);
    });
    return Array.from(groups.values()).sort((a, b) => a.name.localeCompare(b.name));
}

export function hasImpactBranches(symbol) {
    if (!symbol) return false;
    return (symbol.ImpactedRelatedRepos || []).length > 0 || (symbol.ImpactedRepoChains || []).length > 0;
}

// buildImpactHierarchy is buildHierarchy plus one branch per related repo,
// with its matched route(s) beneath it. Branches are marked so the charts can
// draw them in the blue "reached a repository" family rather than as callers.
// A repo reached both directly and through a chain is drawn once.
export function buildImpactHierarchy(symbol) {
    const root = buildHierarchy(symbol || {});
    const branches = new Map();

    function ensureBranch(im) {
        const key = branchKey(im);
        let branch = branches.get(key);
        if (!branch) {
            branch = {
                name: relatedRepoDisplayName(im),
                qualifiedName: `related-repo:${key}`,
                children: [],
                isRelatedRepo: true,
                relatedRepoID: im.RelatedRepoID,
                relatedRepoWebURL: im.RelatedRepoWebURL,
            };
            branches.set(key, branch);
            root.children.push(branch);
        }
        return branch;
    }

    function addRoute(im) {
        if (!im || !im.CallSite) return;
        const branch = ensureBranch(im);
        const label = crossRepoRouteLabel(im.CallSite);
        const qn = `related-route:${branchKey(im)}:${im.CallSite}`;
        if (branch.children.some((child) => child.qualifiedName === qn)) return;
        branch.children.push({
            name: label,
            qualifiedName: qn,
            children: [],
            isRelatedRoute: true,
            isLeaf: true,
            relatedRepoID: im.RelatedRepoID,
            relatedRepoWebURL: im.RelatedRepoWebURL,
        });
    }

    (symbol && symbol.ImpactedRelatedRepos || []).forEach(addRoute);
    (symbol && symbol.ImpactedRepoChains || []).forEach((chain) => {
        (chain.Hops || []).forEach((hop) => {
            addRoute({
                RelatedRepo: hop.RelatedRepo,
                RelatedRepoID: hop.RelatedRepoID,
                RelatedRepoName: hop.RelatedRepoName,
                RelatedRepoWebURL: hop.RelatedRepoWebURL,
                CallSite: hop.CallSite,
                EdgeType: hop.EdgeType,
            });
        });
    });
    return root;
}

// linkRepoText splits server-generated signal detail text on every known
// repository-name variant, returning plain segments and repo references so
// the panel can render the friendly name as a link without duplicating the
// name-matching logic. `matches` is [{start, end, repo}] sorted by position.
export function findRepoMentions(text, impacts) {
    const entries = new Map();
    (impacts || []).forEach((im) => {
        const display = relatedRepoDisplayName(im);
        if (!display) return;
        relatedRepoNameVariants(im).forEach((variant) => {
            if (!entries.has(variant)) entries.set(variant, { display, id: im.RelatedRepoID, webURL: im.RelatedRepoWebURL });
        });
    });
    const names = Array.from(entries.keys()).sort((a, b) => b.length - a.length);
    if (names.length === 0) return [];
    const escaped = names.map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
    const re = new RegExp(`(${escaped.join('|')})`, 'g');
    const mentions = [];
    let m;
    while ((m = re.exec(text)) !== null) {
        const entry = entries.get(m[1]);
        if (entry) mentions.push({ start: m.index, end: m.index + m[1].length, text: m[1], ...entry });
    }
    return mentions;
}
