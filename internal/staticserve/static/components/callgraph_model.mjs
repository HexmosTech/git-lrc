// Pure call-graph presentation logic, kept out of callgraph-utils.js so it
// can be unit tested under `node --test` (the .js modules in this directory
// are browser-only ES modules - node treats them as CommonJS, and they reach
// for window/document/d3). callgraph-utils.js re-exports everything here, so
// chart components keep importing from a single place.

export function shortName(qualifiedName) {
    const parts = (qualifiedName || '').split('.');
    return parts[parts.length - 1] || qualifiedName;
}

// buildHierarchy turns a symbol's flat caller list into the nested shape both
// charts render. Callers arrive with a Path of the intermediate nodes between
// the symbol and themselves (ordered outward from the symbol), so a depth-3
// caller contributes two intermediate levels plus its own leaf.
//
// A caller flagged PreRename references the symbol's *old* name and breaks
// until it's migrated. That flag propagates to every node on its branch, not
// just its leaf: an intermediate only reaches the symbol through the broken
// name, so the whole path is breaking.
export function buildHierarchy(symbol) {
    const callers = symbol.Callers || [];
    const nodeMap = new Map();
    const children = [];
    const childrenNames = new Set();

    function ensureNode(qualifiedName, isIntermediate) {
        if (!nodeMap.has(qualifiedName)) {
            const node = {
                name: shortName(qualifiedName),
                qualifiedName: qualifiedName,
                children: [],
                _names: new Set(),
            };
            if (isIntermediate) node.isIntermediate = true;
            nodeMap.set(qualifiedName, node);
            return node;
        }
        return nodeMap.get(qualifiedName);
    }

    for (const caller of callers) {
        const path = caller.Path || [];

        if (path.length === 0) {
            const node = ensureNode(caller.QualifiedName, false);
            node.depth = caller.Depth;
            node.weight = caller.Weight;
            node.isLeaf = true;
            if (caller.PreRename) node.preRename = true;
            children.push(node);
            childrenNames.add(caller.QualifiedName);
            continue;
        }

        const firstVia = path[0];
        const viaNode = ensureNode(firstVia, true);
        if (!childrenNames.has(firstVia)) {
            children.push(viaNode);
            childrenNames.add(firstVia);
        }
        if (!viaNode.depth) viaNode.depth = 1;
        if (caller.PreRename) viaNode.preRename = true;

        let parent = viaNode;
        for (let i = 1; i < path.length; i++) {
            const childNode = ensureNode(path[i], true);
            if (!parent._names.has(path[i])) {
                parent.children.push(childNode);
                parent._names.add(path[i]);
            }
            if (!childNode.depth) childNode.depth = i + 1;
            if (caller.PreRename) childNode.preRename = true;
            parent = childNode;
        }

        const leafNode = ensureNode(caller.QualifiedName, false);
        leafNode.depth = caller.Depth;
        leafNode.weight = caller.Weight;
        leafNode.isLeaf = true;
        if (caller.PreRename) leafNode.preRename = true;
        if (!parent._names.has(caller.QualifiedName)) {
            parent.children.push(leafNode);
            parent._names.add(caller.QualifiedName);
        }
    }

    return {
        name: symbol.Name || shortName(symbol.QualifiedName),
        qualifiedName: symbol.QualifiedName,
        children,
    };
}

// emptyCallGraphMessage explains *why* a symbol has nothing to draw, which is
// otherwise easy to misread as a broken chart. A renamed symbol is the case
// worth calling out: its post-rename name genuinely has no callers yet, so a
// bare "no callers" reads as a leaf function when it actually means nothing
// else uses the old name either.
export function emptyCallGraphMessage(symbol, view) {
    if (symbol && symbol.RenamedFrom) {
        const newName = symbol.Name || shortName(symbol.QualifiedName);
        if (view === RENAME_VIEW_AFTER) {
            return `Nothing calls "${newName}" yet — renamed from "${symbol.RenamedFrom}"`;
        }
        if (view === RENAME_VIEW_BEFORE) {
            return `Nothing else uses the old name "${symbol.RenamedFrom}"`;
        }
        return `Renamed from "${symbol.RenamedFrom}" — nothing else uses the old name`;
    }
    if (symbol && symbol.Method === 'text-references') {
        return 'Text reference method — no call graph available';
    }
    return 'No callers in the dependency graph';
}

// A renamed symbol has two genuinely different call graphs, and the index only
// knows one of them. "After" is the real thing: CALLS fan-in on the new name,
// as the graph sees the post-change tree - usually empty right after a rename.
// "Before" is who still uses the old name, recovered by text search because
// the old name has no node left to walk from. Keeping them as separate views
// stops the empty After graph from reading as "no callers, nothing to see".
export const RENAME_VIEW_BEFORE = 'before';
export const RENAME_VIEW_AFTER = 'after';

// hasRenameViews reports whether the Before/After split is meaningful for this
// symbol - only renamed symbols have two states to compare.
export function hasRenameViews(symbol) {
    return !!(symbol && symbol.RenamedFrom);
}

// callersForRenameView selects the callers belonging to one side of the split.
// Symbols that were not renamed have a single graph and ignore view entirely.
export function callersForRenameView(symbol, view) {
    const callers = (symbol && symbol.Callers) || [];
    if (!hasRenameViews(symbol)) return callers;
    if (view === RENAME_VIEW_BEFORE) return callers.filter((c) => c.PreRename);
    return callers.filter((c) => !c.PreRename);
}

// symbolForRenameView returns the symbol as the charts should see it for the
// selected view - same symbol, Callers narrowed to that side.
export function symbolForRenameView(symbol, view) {
    if (!hasRenameViews(symbol)) return symbol;
    return { ...symbol, Callers: callersForRenameView(symbol, view) };
}

// groupCallers buckets a caller list for the sidebar. Pre-rename callers get
// their own bucket ahead of the depth buckets rather than being folded into
// "Direct callers": they were found under a different name and are the ones
// worth looking at first, so mixing them into live callers of the same depth
// loses exactly the distinction that matters.
export function groupCallers(callers) {
    const preRename = [];
    const byDepth = new Map();
    (callers || []).forEach((c) => {
        if (c.PreRename) {
            preRename.push(c);
            return;
        }
        const depth = c.Depth || 1;
        if (!byDepth.has(depth)) byDepth.set(depth, []);
        byDepth.get(depth).push(c);
    });

    const groups = [...byDepth.entries()]
        .sort((a, b) => a[0] - b[0])
        .map(([depth, list]) => ({ key: `d${depth}`, depth, preRename: false, callers: list }));

    if (preRename.length > 0) {
        groups.unshift({ key: 'pre-rename', depth: 1, preRename: true, callers: preRename });
    }
    return groups;
}

// callerGroupLabel names a bucket. The pre-rename label deliberately states
// only what was measured - a textual occurrence of the old name inside these
// symbols - and not what it implies. A grep hit can be a real call, a comment,
// or a string literal, so claiming these "break" asserts a compile failure
// that was never verified. oldName is the pre-rename name (symbol.RenamedFrom)
// when known; without it the label stays generic rather than guessing.
export function callerGroupLabel(group, oldName) {
    if (group.preRename) {
        return oldName ? `Still uses the old name "${oldName}"` : 'Still uses the old name';
    }
    if (group.depth === 1) return 'Direct callers';
    return `${group.depth} calls away`;
}
