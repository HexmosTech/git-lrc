import { waitForPreact } from './utils.js';
import { buildHierarchy, DEPTH_COLORS, PRERENAME_COLORS, renderHoverTooltip, emptyCallGraphMessage, loadD3, verifyChartRender } from './callgraph-utils.js';

// renderFlameGraph owns d3 rendering only; tooltip content/visibility is
// reported via onHover(info, x, y) rather than mutated directly on a ref'd
// DOM node - see the matching comment in SunburstChart.js for why.
//
// Returns the number of bars this render expects (see verifyChartRender in
// SunburstChart.js and its use below). immediate=true skips every entrance
// transition and snaps bars straight to their final width/position/opacity
// - used for the automatic repair pass after a verification failure.
function renderFlameGraph(svgEl, symbol, width, height, tooltipRef, onHover, onHoverCaller, immediate, view) {
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
            .text(emptyCallGraphMessage(symbol, view));
        return 0;
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
    function depthGradientId(depth, preRename) {
        const palette = preRename ? PRERENAME_COLORS : DEPTH_COLORS;
        const c = palette[depth] || palette[1];
        const id = (preRename ? 'fg-grad-pre-' : 'fg-grad-') + depth;
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

        const skipAnim = animate && immediate;
        const merged = rects.enter().append('rect')
            .attr('class', 'flame-bar')
            .attr('x', d => d.x).attr('y', leafY)
            .attr('width', d => (animate && !skipAnim) ? 0 : d.width)
            .attr('height', BAR_H)
            .style('opacity', (animate && !skipAnim) ? 0 : 0.92)
            .merge(rects);

        // Fill is per-bar, not per-level: pre-rename callers can sit at the
        // same depth as live ones and must stay visually distinct.
        merged.attr('fill', d => `url(#${depthGradientId(depth + 1, d.node.preRename)})`)
            .attr('rx', 0).attr('ry', 0).attr('cursor', 'pointer')
            .attr('stroke', d => (d.node.preRename ? '#1a2352' : '#3d0000'))
            .attr('stroke-dasharray', d => (d.node.preRename ? '4 2' : null))
            .attr('stroke-width', 1.5);

        if (skipAnim) {
            merged.attr('width', d => d.width).attr('y', y).style('opacity', 0.92);
        } else if (animate) {
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
            onHover(n.isIntermediate
                ? { isIntermediate: true, qualifiedName: n.qualifiedName, depth: n.depth, preRename: !!n.preRename }
                : { isIntermediate: false, name: n.name, qualifiedName: n.qualifiedName, depth: n.depth, preRename: !!n.preRename },
                event.clientX + 14, event.clientY - 10);
            if (onHoverCaller && n.qualifiedName) onHoverCaller(n.qualifiedName);
        })
            .on('mousemove', function (event) {
                if (tooltipRef.current) {
                    tooltipRef.current.style.left = (event.clientX + 14) + 'px';
                    tooltipRef.current.style.top = (event.clientY - 10) + 'px';
                }
            })
            .on('mouseleave', function () {
                d3.select(this).interrupt().transition().duration(100)
                    .style('opacity', 0.92).attr('filter', null);
                onHover(null);
                if (onHoverCaller) onHoverCaller(null);
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

    ensureGradient('fg-grad-root', '#7a0000', '#b30000');
    const rootG = g.append('g');
    rootG.append('rect')
        .attr('x', 0).attr('y', rootY).attr('width', usableW).attr('height', BAR_H)
        .attr('fill', 'url(#fg-grad-root)').attr('rx', 0).attr('ry', 0)
        .attr('stroke', '#6b0000').attr('stroke-width', 1)
        .attr('filter', 'drop-shadow(0 0 6px rgba(255,80,0,0.2)) drop-shadow(0 3px 8px rgba(0,0,0,0.4))');

    rootG.append('text')
        .attr('x', usableW / 2).attr('y', rootY + BAR_H / 2 + 1)
        .attr('text-anchor', 'middle').attr('fill', '#d4a070')
        .attr('font-size', Math.max(9, Math.min(12, BAR_H * 0.55)) + 'px')
        .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", monospace')
        .attr('pointer-events', 'none').text(root.name);

    // All bars across every recursion level are already appended by this
    // point (renderLevel's own recursive calls happen synchronously, before
    // any of their entrance transitions resolve) - only the transitions are
    // still in flight.
    return g.selectAll('rect.flame-bar').size();
}

export async function createFlameGraph() {
    const { html, useState, useRef, useEffect } = await waitForPreact();
    function FlameGraph({ symbol, view, width, height, hoveredCaller, onHoverCaller }) {
        const svgRef = useRef(null); const tooltipRef = useRef(null);
        const [d3Ready, setD3Ready] = useState(typeof window.d3 !== 'undefined');
        const [hover, setHover] = useState(null);
        useEffect(() => {
            let cancelled = false;
            loadD3().then(() => { if (!cancelled) setD3Ready(true); })
                .catch(() => console.warn('D3 load fail'));
            return () => { cancelled = true; };
        }, []);
        // Clear any lingering hover state whenever the selected symbol
        // changes - see the matching comment in SunburstChart.js.
        useEffect(() => { setHover(null); }, [symbol]);
        useEffect(() => {
            if (!d3Ready || !symbol || !svgRef.current) return;
            const el = svgRef.current;
            el.getBoundingClientRect();
            const onHoverCb = (info, x, y) => setHover(info ? { ...info, x, y } : null);
            const timers = [];
            const t = setTimeout(() => {
                if (!el) return;
                const expected = renderFlameGraph(el, symbol, width, height, tooltipRef, onHoverCb, onHoverCaller, false, view);
                // See the matching verification+repair comment in
                // SunburstChart.js - same stalled-transition failure mode,
                // same fix.
                const verifyDelay = Math.max(500, expected * 8 + 450);
                timers.push(setTimeout(() => {
                    if (!el || !el.isConnected) return;
                    if (!verifyChartRender(el, 'rect.flame-bar', expected)) {
                        renderFlameGraph(el, symbol, width, height, tooltipRef, onHoverCb, onHoverCaller, true, view);
                    }
                }, verifyDelay));
            }, 60);
            timers.push(t);
            return () => timers.forEach(clearTimeout);
        }, [d3Ready, symbol, view, width, height]);

        useEffect(() => {
            if (!d3Ready || !svgRef.current) return;
            const svg = window.d3.select(svgRef.current);
            const bars = svg.selectAll('rect.flame-bar');
            if (hoveredCaller) {
                bars.filter(function (d) {
                    return d && d.node && d.node.qualifiedName === hoveredCaller;
                }).interrupt().transition().duration(100)
                    .style('opacity', 1)
                    .attr('filter', 'drop-shadow(0 0 8px rgba(255,152,0,0.5)) drop-shadow(0 0 16px rgba(255,80,0,0.25)) brightness(1.12)');
                bars.filter(function (d) {
                    return !d || !d.node || d.node.qualifiedName !== hoveredCaller;
                }).interrupt().transition().duration(100)
                    .style('opacity', 0.15);
            } else {
                bars.interrupt().transition().duration(150)
                    .style('opacity', 0.92).attr('filter', null);
            }
        }, [hoveredCaller, d3Ready]);
        if (!symbol || (!symbol.Callers || symbol.Callers.length === 0)) {
            return html`<div class="viz-container-fg"><div class="viz-empty">${emptyCallGraphMessage(symbol, view)}</div></div>`;
        }
        return html`
            <div class="viz-container-fg">
                ${hover && html`
                    <div ref=${tooltipRef} class="viz-tooltip" style="left:${hover.x}px;top:${hover.y}px">
                        ${renderHoverTooltip(html, hover, symbol.RenamedFrom)}
                    </div>
                `}
                <svg ref=${svgRef} width=${width} height=${height}></svg>
            </div>
        `;
    }
    return FlameGraph;
}

let FlameGraphComponent = null;
export async function getFlameGraph() {
    if (!FlameGraphComponent) FlameGraphComponent = await createFlameGraph();
    return FlameGraphComponent;
}
