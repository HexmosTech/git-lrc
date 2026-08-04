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
// This must only ever see genuine CALLS-graph callers - the chart draws an
// edge for every entry, so anything synthesized (e.g. a symbol's pre-rename
// callers, found by text search rather than a real graph edge) would render
// as a call that was never actually observed. Callers should filter those out
// before invoking this; see BlastRadiusPanel's chartSymbol.
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

        let parent = viaNode;
        for (let i = 1; i < path.length; i++) {
            const childNode = ensureNode(path[i], true);
            if (!parent._names.has(path[i])) {
                parent.children.push(childNode);
                parent._names.add(path[i]);
            }
            if (!childNode.depth) childNode.depth = i + 1;
            parent = childNode;
        }

        const leafNode = ensureNode(caller.QualifiedName, false);
        leafNode.depth = caller.Depth;
        leafNode.weight = caller.Weight;
        leafNode.isLeaf = true;
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

// emptyCallGraphMessage explains why a symbol has nothing to draw. Kept
// graph-agnostic to rename status on purpose: the chart only ever sees real
// CALLS-graph callers (see buildHierarchy), so whether this symbol was
// renamed is irrelevant to what the *chart* shows - that context belongs in
// the left column's CallerGroup instead (see groupCallers/callerGroupLabel).
export function emptyCallGraphMessage(symbol) {
    if (symbol && symbol.Method === 'text-references') {
        return 'Text reference method — no call graph available';
    }
    return 'No callers in the dependency graph';
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
