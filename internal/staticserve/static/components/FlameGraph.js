import { waitForPreact } from './utils.js';
import { buildHierarchy, DEPTH_COLORS } from './callgraph-utils.js';

function renderFlameGraph(svgEl, symbol, width, height, tooltipEl) {
    const d3 = window.d3;
    const PAD_L = 4;
    const PAD_R = 4;
    const PAD_TB = 8;

    d3.select(svgEl).selectAll('*').remove();

    const svg = d3.select(svgEl)
        .attr('width', width).attr('height', height)
        .attr('viewBox', `0 0 ${width} ${height}`)
        .style('background', 'transparent');

    if (!symbol.Callers || symbol.Callers.length === 0) {
        svg.append('text').attr('x', width / 2).attr('y', height / 2)
            .attr('text-anchor', 'middle').attr('fill', '#8a7060').attr('font-size', '13px')
            .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif')
            .text('No callers in the dependency graph');
        return;
    }

    const root = buildHierarchy(symbol);

    function maxDepth(node) {
        if (!node.children || node.children.length === 0) return 0;
        return 1 + Math.max(...node.children.map(maxDepth));
    }

    function leafSum(node) {
        if (!node.children || node.children.length === 0) return 1;
        return node.children.reduce((s, c) => s + leafSum(c), 0);
    }

    const depthH = maxDepth(root);
    const totalLevels = depthH + 2;
    const usableH = height - PAD_TB * 2;
    const BAR_GAP = Math.max(2, Math.round(usableH * 0.03));
    const BAR_H = Math.max(16, Math.floor((usableH - BAR_GAP * totalLevels) / totalLevels));
    const usableW = width - PAD_L - PAD_R;

    const defs = svg.append('defs');
    function ensureGradient(id, from, to) {
        if (defs.select(`#${id}`).node()) return;
        const grad = defs.append('linearGradient').attr('id', id)
            .attr('x1', '0').attr('y1', '1').attr('x2', '0').attr('y2', '0');
        grad.append('stop').attr('offset', '0%').attr('stop-color', from).attr('stop-opacity', 1);
        grad.append('stop').attr('offset', '100%').attr('stop-color', to).attr('stop-opacity', 1);
    }
    function depthGradientId(depth) {
        const c = DEPTH_COLORS[depth] || DEPTH_COLORS[1];
        const id = 'fg-grad-' + depth;
        ensureGradient(id, c.base, c.light);
        return id;
    }

    const g = svg.append('g').attr('transform', `translate(${PAD_L},0)`);

    function renderLevel(nodes, depth, x0, x1, leafY, animate) {
        const totalW = x1 - x0;
        if (totalW <= 0 || nodes.length === 0) return;

        const totalLeaves = nodes.reduce((s, n) => s + leafSum(n), 0);
        if (totalLeaves === 0) return;

        let cx = x0;
        const bars = [];
        for (const node of nodes) {
            const lc = leafSum(node);
            const w = (lc / totalLeaves) * totalW;
            if (w >= 2) bars.push({ node, x: cx, width: w, depth });
            cx += w;
        }

        const y = leafY - BAR_H;
        const gLevel = g.append('g');

        const rects = gLevel.selectAll('rect.flame-bar')
            .data(bars, d => d.node.qualifiedName);

        rects.exit().interrupt().transition().duration(100)
            .attr('width', 0).attr('y', leafY).style('opacity', 0).remove();

        const merged = rects.enter().append('rect')
            .attr('class', 'flame-bar')
            .attr('x', d => d.x).attr('y', leafY)
            .attr('width', d => animate ? 0 : d.width)
            .attr('height', BAR_H)
            .style('opacity', animate ? 0 : 0.92)
            .merge(rects);

        const gradId = depthGradientId(depth + 1);
        merged.attr('fill', `url(#${gradId})`).attr('rx', 0).attr('ry', 0).attr('cursor', 'pointer')
            .attr('stroke', '#1a0a00').attr('stroke-width', 1.5);

        if (animate) {
            merged.interrupt().transition().duration(300).delay((d, i) => i * 8)
                .attr('width', d => d.width).attr('y', y).style('opacity', 0.92);
        } else {
            merged.interrupt().transition().duration(200).attr('y', y);
        }

        merged.on('mouseenter', function (event, d) {
            d3.select(this).interrupt().transition().duration(60)
                .style('opacity', 1)
                .attr('filter', 'drop-shadow(0 0 8px rgba(255,152,0,0.5)) drop-shadow(0 0 16px rgba(255,80,0,0.25)) brightness(1.12)');
            const n = d.node;
            tooltipEl.innerHTML = n.isIntermediate
                ? `<strong>&#8627;</strong> ${n.qualifiedName}<br><span style="color:#8a7060;font-size:10px">intermediate</span>`
                : `<strong>${n.name}</strong><br><span style="color:#baa090;font-size:10px">${n.qualifiedName}</span><br><span style="color:#8a7060;font-size:10px">${n.depth === 1 ? 'Direct caller' : n.depth + ' hops away'}</span>`;
            tooltipEl.style.display = 'block'; tooltipEl.style.opacity = '1';
        })
            .on('mousemove', function (event) {
                tooltipEl.style.left = (event.clientX + 14) + 'px';
                tooltipEl.style.top = (event.clientY - 10) + 'px';
            })
            .on('mouseleave', function () {
                d3.select(this).interrupt().transition().duration(100)
                    .style('opacity', 0.92).attr('filter', null);
                tooltipEl.style.display = 'none'; tooltipEl.style.opacity = '0';
            });

        for (const bar of bars) {
            if (bar.node.children && bar.node.children.length > 0) {
                renderLevel(bar.node.children, depth + 1, bar.x, bar.x + bar.width, y - BAR_GAP, animate);
            }
        }
    }

    const bottomY = height - PAD_TB;
    const rootY = bottomY - BAR_H;

    renderLevel(root.children, 0, 0, usableW, rootY - BAR_GAP, true);

    ensureGradient('fg-grad-root', '#2d0000', '#6a1000');
    const rootG = g.append('g');
    rootG.append('rect')
        .attr('x', 0).attr('y', rootY).attr('width', usableW).attr('height', BAR_H)
        .attr('fill', 'url(#fg-grad-root)').attr('rx', 2).attr('ry', 2)
        .attr('stroke', '#4a1000').attr('stroke-width', 0.5)
        .attr('filter', 'drop-shadow(0 0 6px rgba(255,80,0,0.2)) drop-shadow(0 3px 8px rgba(0,0,0,0.4))');

    rootG.append('text')
        .attr('x', usableW / 2).attr('y', rootY + BAR_H / 2 + 1)
        .attr('text-anchor', 'middle').attr('fill', '#d4a070')
        .attr('font-size', Math.max(9, Math.min(12, BAR_H * 0.55)) + 'px')
        .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", monospace')
        .attr('pointer-events', 'none').text(root.name);
}

export async function createFlameGraph() {
    const { html, useState, useRef, useEffect } = await waitForPreact();
    function FlameGraph({ symbol, width, height }) {
        const svgRef = useRef(null); const tooltipRef = useRef(null);
        const [d3Ready, setD3Ready] = useState(typeof window.d3 !== 'undefined');
        useEffect(() => {
            if (window.d3) { setD3Ready(true); return; }
            const s = document.createElement('script'); s.src = '/static/vendor/d3.v7.min.js';
            s.onload = () => setD3Ready(true); s.onerror = () => console.warn('D3 load fail');
            document.head.appendChild(s);
        }, []);
        useEffect(() => {
            if (!d3Ready || !symbol || !svgRef.current || !tooltipRef.current) return;
            const el = svgRef.current;
            const tt = tooltipRef.current;
            el.getBoundingClientRect();
            const t = setTimeout(() => {
                if (el && tt) renderFlameGraph(el, symbol, width, height, tt);
            }, 60);
            return () => clearTimeout(t);
        }, [d3Ready, symbol, width, height]);
        if (!symbol || (!symbol.Callers || symbol.Callers.length === 0)) {
            return html`<div class="viz-container-fg"><div class="viz-empty">${symbol && symbol.Method === 'text-references' ? 'Text reference method — no call graph available' : 'No callers in the dependency graph'}</div></div>`;
        }
        return html`<div class="viz-container-fg"><div ref=${tooltipRef} class="viz-tooltip" style="display:none"></div><svg ref=${svgRef} width=${width} height=${height}></svg></div>`;
    }
    return FlameGraph;
}

let FlameGraphComponent = null;
export async function getFlameGraph() {
    if (!FlameGraphComponent) FlameGraphComponent = await createFlameGraph();
    return FlameGraphComponent;
}
