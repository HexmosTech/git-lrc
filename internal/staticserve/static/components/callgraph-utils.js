export function shortName(qualifiedName) {
    const parts = (qualifiedName || '').split('.');
    return parts[parts.length - 1] || qualifiedName;
}

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

export const DEPTH_COLORS = {
    0: { base: '#990000', light: '#e60000' },
    1: { base: '#ff1744', light: '#ff616f' },
    2: { base: '#ff5722', light: '#ff8a65' },
    3: { base: '#ffab00', light: '#ffe082' },
    4: { base: '#ffd600', light: '#ffff8d' },
};

export function interpolateColor(c1, c2, t) {
    const r1 = parseInt(c1.slice(1, 3), 16);
    const g1 = parseInt(c1.slice(3, 5), 16);
    const b1 = parseInt(c1.slice(5, 7), 16);
    const r2 = parseInt(c2.slice(1, 3), 16);
    const g2 = parseInt(c2.slice(3, 5), 16);
    const b2 = parseInt(c2.slice(5, 7), 16);
    const r = Math.round(r1 + (r2 - r1) * t);
    const g = Math.round(g1 + (g2 - g1) * t);
    const b = Math.round(b1 + (b2 - b1) * t);
    return `rgb(${r},${g},${b})`;
}

export function getDepthColor(d) {
    if (d.depth === 0) return (DEPTH_COLORS[0] || { base: '#12121c' }).base;
    const ring = DEPTH_COLORS[d.depth] || { base: '#607d8b', light: '#90a4ae' };
    if (d.parent && d.parent.children && d.parent.children.length > 1) {
        const idx = d.parent.children.indexOf(d);
        if (idx >= 0) {
            const frac = idx / Math.max(d.parent.children.length - 1, 1);
            return interpolateColor(ring.base, ring.light, frac);
        }
    }
    return ring.base;
}

export function countLeaves(node) {
    if (!node.children || node.children.length === 0) return 1;
    return node.children.reduce((sum, c) => sum + countLeaves(c), 0);
}

// hoverInfoFromDatum builds the plain hover-state object (name/qualifiedName/
// depth/isIntermediate) from a d3 hierarchy datum's `.data`, shared by
// SunburstChart and FlameGraph so their tooltip content can't drift apart -
// both charts build hierarchies from the same buildHierarchy() shape above.
export function hoverInfoFromDatum(d) {
    const b = d.data;
    if (b.isIntermediate) {
        return { isIntermediate: true, qualifiedName: b.qualifiedName, depth: d.depth };
    }
    return { isIntermediate: false, name: b.name, qualifiedName: b.qualifiedName, depth: b.depth };
}

// renderHoverTooltip returns the vdom content for a hover-state object (see
// hoverInfoFromDatum) - `html` is the caller's own htm-bound tag (each
// component gets its own instance from waitForPreact(), so it's passed in
// rather than imported here).
export function renderHoverTooltip(html, hover) {
    if (!hover) return null;
    if (hover.isIntermediate) {
        return html`<strong>&#8627;</strong> ${hover.qualifiedName}<br /><span style="color:#9a8070;font-size:10px">intermediate &middot; depth ${hover.depth}</span>`;
    }
    return html`<strong>${hover.name}</strong><br /><span style="color:#baa090;font-size:10px">${hover.qualifiedName}</span><br /><span style="color:#8a7060;font-size:10px">${hover.depth === 1 ? 'Direct caller' : hover.depth + ' hops away'}</span>`;
}
