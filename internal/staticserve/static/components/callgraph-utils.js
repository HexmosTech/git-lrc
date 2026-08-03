export function shortName(qualifiedName) {
    const parts = (qualifiedName || '').split('.');
    return parts[parts.length - 1] || qualifiedName;
}

export function buildHierarchy(symbol) {
    const callers = symbol.Callers || [];
    const nodeMap = new Map();
    const children = [];

    function ensureNode(qualifiedName, isIntermediate) {
        if (!nodeMap.has(qualifiedName)) {
            const node = {
                name: shortName(qualifiedName),
                qualifiedName: qualifiedName,
                children: [],
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
            continue;
        }

        const firstVia = path[0];
        const viaNode = ensureNode(firstVia, true);
        if (!children.some(c => c.qualifiedName === firstVia)) {
            children.push(viaNode);
        }
        if (!viaNode.depth) viaNode.depth = 1;

        let parent = viaNode;
        for (let i = 1; i < path.length; i++) {
            const childNode = ensureNode(path[i], true);
            if (!parent.children.some(c => c.qualifiedName === path[i])) {
                parent.children.push(childNode);
            }
            if (!childNode.depth) childNode.depth = i + 1;
            parent = childNode;
        }

        const leafNode = ensureNode(caller.QualifiedName, false);
        leafNode.depth = caller.Depth;
        leafNode.weight = caller.Weight;
        leafNode.isLeaf = true;
        if (!parent.children.some(c => c.qualifiedName === caller.QualifiedName)) {
            parent.children.push(leafNode);
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
    if (d.depth === 0) return '#12121c';
    const ring = DEPTH_COLORS[d.depth] || { base: '#607d8b', light: '#90a4ae' };
    if (d.parent && d.parent.children && d.parent.children.length > 1) {
        const idx = d.parent.children.indexOf(d);
        const frac = idx / (d.parent.children.length - 1);
        if (frac >= 0) return interpolateColor(ring.base, ring.light, frac);
    }
    return ring.base;
}

export function countLeaves(node) {
    if (!node.children || node.children.length === 0) return 1;
    return node.children.reduce((sum, c) => sum + countLeaves(c), 0);
}
