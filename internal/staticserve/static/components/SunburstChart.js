import { waitForPreact } from './utils.js';
import { buildHierarchy, getDepthColor, DEPTH_COLORS, PRERENAME_COLORS, hoverInfoFromDatum, renderHoverTooltip, emptyCallGraphMessage, loadD3, verifyChartRender } from './callgraph-utils.js';

function arcTween(a, arcGen) {
    const i = window.d3.interpolate({ x0: a.x0, x1: a.x0 }, a);
    return t => arcGen(i(t));
}

// renderSunburst owns d3 rendering only. Tooltip CONTENT/visibility is
// reported via onHover(info, x, y) (info null on mouseleave) so Preact's own
// vdom model of the tooltip's children always matches what's on screen -
// mutating a ref'd node's innerHTML directly (the previous approach) left
// stale content behind whenever Preact later reused that DOM node for a
// different vdom shape (e.g. switching to the "no call graph" empty state),
// since Preact never tracked the imperatively-injected children. Only the
// tooltip's *position* stays imperative (via tooltipRef, updated on every
// mousemove) - that's a leaf style write, not content, so it can't desync.
//
// Returns the number of arcs this render expects to end up with, so the
// caller can verify the draw actually completed (see verifyChartRender in
// the component below) - a fresh call always clears the SVG first, so every
// element is in the "enter" branch below regardless of what was drawn
// before.
//
// immediate=true skips the entrance transition and writes final d/opacity
// directly - used for the automatic repair pass after a verification
// failure, so the retry can't itself get caught by whatever stalled the
// first attempt (a slow/backgrounded tab throttling the transition's timer
// ticks, concurrent chart mounts competing for the main thread, etc).
function renderSunburst(svgEl, symbol, width, height, tooltipRef, onHover, onHoverCaller, immediate, view) {
    const d3 = window.d3;
    const radius = Math.min(width, height) / 2;

    d3.select(svgEl).selectAll('*').remove();

    const svg = d3.select(svgEl)
        .attr('width', width).attr('height', height)
        .attr('viewBox', `0 0 ${width} ${height}`)
        .style('background', 'transparent');

    const defs = svg.append('defs');
    const addGradient = (id, c) => {
        const grad = defs.append('linearGradient')
            .attr('id', id)
            .attr('x1', '0').attr('y1', '1')
            .attr('x2', '0').attr('y2', '0');
        grad.append('stop').attr('offset', '0%').attr('stop-color', c.base).attr('stop-opacity', 1);
        grad.append('stop').attr('offset', '60%').attr('stop-color', c.light).attr('stop-opacity', 0.95);
        grad.append('stop').attr('offset', '100%').attr('stop-color', c.light).attr('stop-opacity', 0.7);
    };
    for (let d = 1; d <= 4; d++) {
        if (DEPTH_COLORS[d]) addGradient(`sb-grad-${d}`, DEPTH_COLORS[d]);
        if (PRERENAME_COLORS[d]) addGradient(`sb-grad-pre-${d}`, PRERENAME_COLORS[d]);
    }

    const g = svg.append('g')
        .attr('transform', `translate(${width / 2},${height / 2})`);

    if (!symbol.Callers || symbol.Callers.length === 0) {
        g.append('text').attr('text-anchor', 'middle').attr('dy', '0.35em')
            .attr('fill', '#8a7060').attr('font-size', '13px')
            .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif')
            .text(emptyCallGraphMessage(symbol, view));
        return 0;
    }

    const root = buildHierarchy(symbol);
    const hierarchy = d3.hierarchy(root)
        .sum(d => (d.children && d.children.length > 0) ? 0 : 1)
        .sort((a, b) => b.value - a.value);

    const partition = d3.partition().size([2 * Math.PI, radius]);
    partition(hierarchy);

    const arc = d3.arc()
        .startAngle(d => d.x0).endAngle(d => d.x1)
        .innerRadius(d => d.y0).outerRadius(d => d.y1)
        .padAngle(0.005).padRadius(radius * 0.55);

    const visible = hierarchy.descendants().filter(d => d.depth > 0);

    g.selectAll('path.sunburst-arc')
        .data(visible, d => {
            const parts = [];
            let n = d; while (n.depth > 0) { parts.unshift(n.data.qualifiedName || n.data.name); n = n.parent; }
            return parts.join('|');
        })
        .join(
            enter => immediate
                ? enter.append('path').attr('class', 'sunburst-arc')
                    .attr('d', d => arc(d))
                    .style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78))
                : enter.append('path').attr('class', 'sunburst-arc')
                    .attr('d', d => { const m = (d.x0 + d.x1) / 2; return arc({ x0: m, x1: m, y0: d.y0, y1: d.y0 }); })
                    .style('opacity', 0)
                    .call(sel => sel.interrupt().transition().duration(400).delay((d, i) => i * 8)
                        .attrTween('d', d => arcTween(d, arc))
                        .style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78)),
                    ),
            update => update.call(sel => sel.interrupt().transition().duration(300)
                .attr('d', d => arc(d)).style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78))),
            exit => exit.interrupt().transition().duration(150)
                .attrTween('d', d => {
                    const m = (d.x0 + d.x1) / 2;
                    const i = d3.interpolate({ x0: d.x0, x1: d.x1, y0: d.y0, y1: d.y1 }, { x0: m, x1: m, y0: d.y0, y1: d.y0 });
                    return t => arc(i(t));
                })
                .style('opacity', 0).remove(),
        );

    g.selectAll('path.sunburst-arc')
        .attr('fill', d => (d.data.preRename ? `url(#sb-grad-pre-${d.depth})` : `url(#sb-grad-${d.depth})`))
        .attr('stroke', d => (d.data.preRename ? '#1a2352' : '#4a0000')).attr('stroke-width', 0.7)
        .attr('stroke-dasharray', d => (d.data.preRename ? '3 2' : null))
        .attr('cursor', 'pointer')
        .on('mouseenter', function (event, d) {
            d3.select(this).interrupt().transition().duration(80)
                .style('opacity', 1).attr('stroke', '#fff7e0').attr('stroke-width', 1.8)
                .attr('filter', 'drop-shadow(0 0 10px rgba(255,152,0,0.5)) drop-shadow(0 0 20px rgba(255,100,0,0.3))');
            g.selectAll('path.sunburst-arc').interrupt().transition().duration(80)
                .style('opacity', p => (p === d || p === d.parent) ? 1 : 0.12);
            const b = d.data;
            onHover(hoverInfoFromDatum(d), event.clientX + 16, event.clientY - 12);
            if (onHoverCaller && b.qualifiedName) onHoverCaller(b.qualifiedName);
        })
        .on('mousemove', function (event) {
            if (tooltipRef.current) {
                tooltipRef.current.style.left = (event.clientX + 16) + 'px';
                tooltipRef.current.style.top = (event.clientY - 12) + 'px';
            }
        })
        .on('mouseleave', function () {
            g.selectAll('path.sunburst-arc').transition().duration(150)
                .style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78))
                .attr('stroke', d => (d.data.preRename ? '#1a2352' : '#4a0000'))
                .attr('stroke-width', 0.7).attr('filter', null);
            onHover(null);
            if (onHoverCaller) onHoverCaller(null);
        });

    const centerR = radius * 0.08;
    g.append('circle').attr('r', centerR + 3).attr('fill', '#7a0000').attr('stroke', '#990000')
        .attr('stroke-width', 1.5)
        .attr('filter', 'drop-shadow(0 0 6px rgba(255,100,0,0.3)) drop-shadow(0 0 14px rgba(255,60,0,0.15))');
    g.append('text').attr('text-anchor', 'middle').attr('dy', '-0.2em')
        .attr('fill', '#d4a070').attr('font-size', '9px')
        .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", monospace')
        .text(root.name);
    g.append('text').attr('text-anchor', 'middle').attr('y', centerR + 14)
        .attr('fill', '#8a6040').attr('font-size', '8px')
        .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif')
        .text(root.children.length + ' child' + (root.children.length !== 1 ? 'ren' : ''));

    return visible.length;
}

export async function createSunburstChart() {
    const { html, useState, useRef, useEffect } = await waitForPreact();
    function SunburstChart({ symbol, view, width, height, hoveredCaller, onHoverCaller }) {
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
        // changes - defense-in-depth so switching symbols via the nav (e.g.
        // to one with no call graph at all) can never leave a previous
        // symbol's tooltip content visible.
        useEffect(() => { setHover(null); }, [symbol]);
        useEffect(() => {
            if (!d3Ready || !symbol || !svgRef.current) return;
            const el = svgRef.current;
            el.getBoundingClientRect();
            const onHoverCb = (info, x, y) => setHover(info ? { ...info, x, y } : null);
            const timers = [];
            const t = setTimeout(() => {
                if (!el) return;
                const expected = renderSunburst(el, symbol, width, height, tooltipRef, onHoverCb, onHoverCaller, false, view);
                // Verify once the entrance transition should have finished
                // (last arc's delay + its own duration, plus a grace
                // margin) - if it stalled or got interrupted (backgrounded
                // tab throttling the transition clock, a concurrent chart
                // mount competing for the main thread, etc - see loadD3),
                // redraw immediately, no animation, guaranteed complete.
                const verifyDelay = Math.max(500, expected * 8 + 450);
                timers.push(setTimeout(() => {
                    if (!el || !el.isConnected) return;
                    if (!verifyChartRender(el, 'path.sunburst-arc', expected)) {
                        renderSunburst(el, symbol, width, height, tooltipRef, onHoverCb, onHoverCaller, true, view);
                    }
                }, verifyDelay));
            }, 60);
            timers.push(t);
            return () => timers.forEach(clearTimeout);
        }, [d3Ready, symbol, view, width, height]);

        useEffect(() => {
            if (!d3Ready || !svgRef.current) return;
            const svg = window.d3.select(svgRef.current);
            const arcs = svg.selectAll('path.sunburst-arc');
            if (hoveredCaller) {
                arcs.filter(function (d) {
                    return d && d.data && d.data.qualifiedName === hoveredCaller;
                }).interrupt().transition().duration(100)
                    .style('opacity', 1)
                    .attr('stroke', '#fff7e0').attr('stroke-width', 1.8)
                    .attr('filter', 'drop-shadow(0 0 10px rgba(255,152,0,0.5)) drop-shadow(0 0 20px rgba(255,100,0,0.3))');
                arcs.filter(function (d) {
                    return !d || !d.data || d.data.qualifiedName !== hoveredCaller;
                }).interrupt().transition().duration(100)
                    .style('opacity', 0.12);
            } else {
                arcs.interrupt().transition().duration(150)
                    .style('opacity', d => (d && d.data && d.data.isLeaf ? 0.92 : 0.78))
                    .attr('stroke', d => (d && d.data && d.data.preRename ? '#1a2352' : '#4a0000'))
                    .attr('stroke-width', 0.7).attr('filter', null);
            }
        }, [hoveredCaller, d3Ready]);
        if (!symbol || (!symbol.Callers || symbol.Callers.length === 0)) {
            return html`<div class="viz-container-sq"><div class="viz-empty">${emptyCallGraphMessage(symbol, view)}</div></div>`;
        }
        return html`
            <div class="viz-container-sq">
                ${hover && html`
                    <div ref=${tooltipRef} class="viz-tooltip" style="left:${hover.x}px;top:${hover.y}px">
                        ${renderHoverTooltip(html, hover, symbol.RenamedFrom)}
                    </div>
                `}
                <svg ref=${svgRef} width=${width} height=${height}></svg>
            </div>
        `;
    }
    return SunburstChart;
}

let SunburstChartComponent = null;
export async function getSunburstChart() {
    if (!SunburstChartComponent) SunburstChartComponent = await createSunburstChart();
    return SunburstChartComponent;
}
