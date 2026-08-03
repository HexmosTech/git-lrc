import { waitForPreact } from './utils.js';
import { buildHierarchy, getDepthColor } from './callgraph-utils.js';

function arcTween(a, arcGen) {
    const i = window.d3.interpolate({ x0: a.x0, x1: a.x0 }, a);
    return t => arcGen(i(t));
}

function renderSunburst(svgEl, symbol, width, height, tooltipEl) {
    const d3 = window.d3;
    const radius = Math.min(width, height) / 2;

    d3.select(svgEl).selectAll('*').remove();

    const svg = d3.select(svgEl)
        .attr('width', width).attr('height', height)
        .attr('viewBox', `0 0 ${width} ${height}`)
        .style('background', 'transparent');

    const g = svg.append('g')
        .attr('transform', `translate(${width / 2},${height / 2})`);

    if (!symbol.Callers || symbol.Callers.length === 0) {
        g.append('text').attr('text-anchor', 'middle').attr('dy', '0.35em')
            .attr('fill', '#8a7060').attr('font-size', '13px')
            .attr('font-family', '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif')
            .text('No callers in the dependency graph');
        return;
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
            enter => enter.append('path').attr('class', 'sunburst-arc')
                .attr('d', d => { const m = (d.x0 + d.x1) / 2; return arc({ x0: m, x1: m, y0: d.y0, y1: d.y0 }); })
                .style('opacity', 0)
                .call(sel => sel.interrupt().transition().duration(400).delay((d, i) => i * 8)
                    .attrTween('d', d => arcTween(d, arc))
                    .style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78)),
                ),
            update => update.call(sel => sel.interrupt().transition().duration(300)
                .attr('d', d => arc(d)).style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78))),
            exit => exit.interrupt().transition().duration(150)
                .attrTween('d', d => { const m = (d.x0 + d.x1) / 2; return t => arc({ x0: m, x1: m, y0: d.y0, y1: d.y0 }); })
                .style('opacity', 0).remove(),
        );

    g.selectAll('path.sunburst-arc')
        .attr('fill', d => getDepthColor(d))
        .attr('stroke', '#1a0000').attr('stroke-width', 0.8)
        .attr('cursor', 'pointer')
        .on('mouseenter', function (event, d) {
            d3.select(this).interrupt().transition().duration(80)
                .style('opacity', 1).attr('stroke', '#fff7e0').attr('stroke-width', 1.8)
                .attr('filter', 'drop-shadow(0 0 10px rgba(255,152,0,0.5)) drop-shadow(0 0 20px rgba(255,100,0,0.3))');
            g.selectAll('path.sunburst-arc').interrupt().transition().duration(80)
                .style('opacity', p => (p === d || p === d.parent || (d.parent && p === d.parent)) ? 1 : 0.12);
            const b = d.data;
            tooltipEl.innerHTML = b.isIntermediate
                ? `<strong>&#8627;</strong> ${b.qualifiedName}<br><span style="color:#9a8070;font-size:10px">intermediate &middot; depth ${d.depth}</span>`
                : `<strong>${b.name}</strong><br><span style="color:#baa090;font-size:10px">${b.qualifiedName}</span><br><span style="color:#8a7060;font-size:10px">${b.depth === 1 ? 'Direct caller' : b.depth + ' hops away'}</span>`;
            tooltipEl.style.display = 'block'; tooltipEl.style.opacity = '1';
        })
        .on('mousemove', function (event) {
            tooltipEl.style.left = (event.clientX + 16) + 'px';
            tooltipEl.style.top = (event.clientY - 12) + 'px';
        })
        .on('mouseleave', function () {
            g.selectAll('path.sunburst-arc').transition().duration(150)
                .style('opacity', d => (d.data.isLeaf ? 0.92 : 0.78))
                .attr('stroke', '#1a0000').attr('stroke-width', 0.8).attr('filter', null);
            tooltipEl.style.display = 'none'; tooltipEl.style.opacity = '0';
        });

    const centerR = radius * 0.08;
    g.append('circle').attr('r', centerR + 3).attr('fill', '#2d0000').attr('stroke', '#4a1000')
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
}

export async function createSunburstChart() {
    const { html, useState, useRef, useEffect } = await waitForPreact();
    function SunburstChart({ symbol, width, height }) {
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
            // Force layout so SVG has final dimensions before D3 renders.
            el.getBoundingClientRect();
            const t = setTimeout(() => {
                if (el && tt) renderSunburst(el, symbol, width, height, tt);
            }, 60);
            return () => clearTimeout(t);
        }, [d3Ready, symbol, width, height]);
        if (!symbol || (!symbol.Callers || symbol.Callers.length === 0)) {
            return html`<div class="viz-container-sq"><div class="viz-empty">${symbol && symbol.Method === 'text-references' ? 'Text reference method — no call graph available' : 'No callers in the dependency graph'}</div></div>`;
        }
        return html`<div class="viz-container-sq"><div ref=${tooltipRef} class="viz-tooltip" style="display:none"></div><svg ref=${svgRef} width=${width} height=${height}></svg></div>`;
    }
    return SunburstChart;
}

let SunburstChartComponent = null;
export async function getSunburstChart() {
    if (!SunburstChartComponent) SunburstChartComponent = await createSunburstChart();
    return SunburstChartComponent;
}
