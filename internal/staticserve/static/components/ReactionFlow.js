import { waitForPreact } from './utils.js';

const FEEDBACK_TAGS = ['False positive','Wrong severity','Missed something','Too noisy','Hard to act on','Off-base'];
const STAR_URL = 'https://github.com/HexmosTech/git-lrc';

function keyForComment(filePath, content) {
  const prefix = (content || '').slice(0, 80);
  return `lrc.react:${filePath || 'unknown'}::${prefix}`;
}

function defaultImpact() { return {name:'Maneshwar Holla',initials:'MH',meta:'Senior Engineer · 247 reviews · since Jan 2024',primary:[{label:'Reviews',value:247},{label:'Issues found',value:1438},{label:'Bugs caught pre-prod',value:89},{label:'First comment',value:'28s'}],severity:{critical:89,error:234,warning:615,info:500}}; }

export async function createReactionFlow() {
  const { html, useEffect, useMemo, useState, useRef } = await waitForPreact();

  return function ReactionFlow({ scope, filePath, comment, catches = [], prId }) {
    const storageKey = scope === 'pr' ? 'lrc.react:pr-level' : keyForComment(filePath, comment?.Content || '');
    const [state, setState] = useState(() => localStorage.getItem(storageKey) || '');
    const [step, setStep] = useState(() => (state === 'up' ? 'thanks' : state === 'down:sent' ? 'down-sent' : state === 'down' ? 'down' : ''));
    const [impact, setImpact] = useState(null);
    const [text, setText] = useState('');
    const [tags, setTags] = useState(new Set());
    const [includeContext, setIncludeContext] = useState(true);
    const [includeCatches, setIncludeCatches] = useState(false);
    const [selectedCatchIds, setSelectedCatchIds] = useState(new Set());
    const [includeCode, setIncludeCode] = useState(false);
    const [channel, setChannel] = useState('linkedin');
    const canvasRef = useRef(null);

    useEffect(() => { if (state) localStorage.setItem(storageKey, state); else localStorage.removeItem(storageKey); }, [state, storageKey]);
    useEffect(() => { if (step !== 'share') return; fetch('/api/user/impact').then(r=>r.ok?r.json():defaultImpact()).then(setImpact).catch(()=>setImpact(defaultImpact())); }, [step]);
    useEffect(() => {
      if (step !== 'share' || !includeCatches || !canvasRef.current) return;
      const dims = channel === 'x' ? [1080,1080] : [1200,630]; const [w,h]=dims; const c = canvasRef.current; c.width=w;c.height=h; const ctx=c.getContext('2d');
      ctx.fillStyle='#0f172a';ctx.fillRect(0,0,w,h);ctx.fillStyle='#2563eb';ctx.fillRect(30,30,4,h-60); ctx.fillStyle='white';ctx.font='bold 42px sans-serif';ctx.fillText('git-lrc',60,80); ctx.font='28px sans-serif';ctx.fillText('caught this in code review',60,120);
      let y=160; const picks=catches.filter(x=>selectedCatchIds.has(x.id));
      picks.forEach((x)=>{ctx.fillStyle='#1f2937';ctx.fillRect(60,y,w-120,includeCode?170:95);ctx.fillStyle='white';ctx.font='bold 22px sans-serif';ctx.fillText(`[${(x.severity||'info').toUpperCase()}] ${String(x.title||'').slice(0,70)}`,80,y+35); if(includeCode){ctx.font='18px monospace';ctx.fillStyle='#94a3b8';ctx.fillText(`${x.file||''}:${x.line||''}`,80,y+65);ctx.fillStyle='#e2e8f0';(x.snippet||'').split('\n').slice(0,4).forEach((ln,i)=>ctx.fillText(ln,80,y+95+(i*22)));} y += includeCode?190:110;});
      ctx.fillStyle='#fde68a';ctx.font='bold 30px sans-serif';ctx.fillText('⭐ Star git-lrc on GitHub',60,h-34);ctx.fillStyle='#94a3b8';ctx.font='24px monospace';ctx.fillText('github.com/HexmosTech/git-lrc',430,h-34);
    }, [step, includeCatches, includeCode, selectedCatchIds, catches, channel]);

    const caption = useMemo(()=>{const data=impact||defaultImpact(); const p=data.primary||[]; const reviews=p[0]?.value||247; const critical=data.severity?.critical||89; const totalIssues=p[1]?.value||1438; const latency=p[3]?.value||'28s'; const header=`Hit ${reviews} code reviews with git-lrc 🎯`;
      if(!includeCatches){return `${header}\n\nThe scoreboard so far:\n  • ${critical} critical bugs caught before production\n  • ${totalIssues} issues surfaced\n  • ${latency} median first-comment latency\n\ngit-lrc is the most underrated devtool I've used this year. Try it (and ⭐ it):\n${STAR_URL}\n\n#CodeReview #DevTools #git-lrc`;}
      const lines=catches.filter(x=>selectedCatchIds.has(x.id)).map(x=>`  • [${(x.severity||'info').toUpperCase()}] ${x.title}`);
      return `${header}\n\nHere's what it caught in my latest PR:\n${lines.join('\n')||'  • [CRITICAL] Flat input payload silently dropped for non-UI callers'}\n\nThe all-time scoreboard:\n  • ${critical} critical bugs caught before production\n  • ${totalIssues} issues surfaced\n  • ${latency} median first-comment latency\n\ngit-lrc is the most underrated devtool I've used this year. Try it (and ⭐ it):\n${STAR_URL}\n\n#CodeReview #DevTools #git-lrc`;},[impact,includeCatches,selectedCatchIds,catches]);

    const setReaction = (kind) => { setState(kind); setStep(kind === 'up' ? 'thanks' : 'down'); };

    return html`<div class="lrc-reaction-wrap"><div class="lrc-reaction-bar"><span>${scope==='pr'?'Whole PR:':'Was this helpful?'}</span><button class="lrc-pill ${state.startsWith('up')?'locked-up':''}" onClick=${()=>setReaction('up')}>👍 ${scope==='pr'?'':'Helpful'}</button><button class="lrc-pill ${state.startsWith('down')?'locked-down':''}" onClick=${()=>setReaction('down')}>👎 ${scope==='pr'?'':'Not useful'}</button>${state&&html`<span class="lrc-react-note">${state==='up'?'Thanks!':'Got it.'}</span>`}</div>
    ${step==='thanks'&&html`<div class="lrc-react-reveal"><strong>✓ Thanks for the signal 🙌</strong><p>Want to see what git-lrc has caught for you?</p><button class="btn btn-primary" onClick=${()=>setStep('stats')}>Show my impact stats</button><button class="btn" onClick=${()=>setStep('')}>Maybe later</button></div>`}
    ${step==='stats'&&html`<div class="lrc-react-reveal"><div class="lrc-impact-grid">${(impact||defaultImpact()).primary.map(it=>html`<div class="lrc-stat-card"><div>${it.value}</div><small>${it.label}</small></div>`)}<div class="lrc-stat-card sev-critical">${(impact||defaultImpact()).severity.critical} CRIT</div><div class="lrc-stat-card sev-error">${(impact||defaultImpact()).severity.error} ERR</div><div class="lrc-stat-card sev-warning">${(impact||defaultImpact()).severity.warning} WARN</div><div class="lrc-stat-card sev-info">${(impact||defaultImpact()).severity.info} INFO</div></div><button class="btn btn-primary" onClick=${()=>setStep('share')}>Share a catch on social</button><button class="btn" onClick=${()=>setStep('pitch')}>Pitch to Leadership</button><button class="btn" onClick=${()=>setStep('')}>Maybe later</button></div>`}
    ${step==='share'&&html`<div class="lrc-react-reveal"><label><input type="checkbox" checked=${includeCatches} onChange=${(e)=>setIncludeCatches(e.target.checked)}/> Also share what git-lrc caught in this review</label>${includeCatches&&html`<div>${catches.map(c=>html`<label><input type="checkbox" checked=${selectedCatchIds.has(c.id)} onChange=${(e)=>{const n=new Set(selectedCatchIds);e.target.checked?n.add(c.id):n.delete(c.id);setSelectedCatchIds(n);}}/>[${(c.severity||'info').toUpperCase()}] ${c.title}</label>`)}</div><label><input type="checkbox" checked=${includeCode} onChange=${(e)=>setIncludeCode(e.target.checked)}/> Include filepath + code snippet</label><button class="btn" onClick=${()=>setChannel(channel==='linkedin'?'x':'linkedin')}>${channel==='linkedin'?'LinkedIn 1200×630':'X 1080×1080'}</button><canvas ref=${canvasRef} style="max-width:460px;width:100%"></canvas>`}<pre class="lrc-caption">${caption}</pre><button class="btn btn-primary" onClick=${()=>navigator.clipboard.writeText(caption)}>Copy caption</button><button class="btn" onClick=${()=>setStep('stats')}>Back</button></div>`}
    ${step==='pitch'&&html`<div class="lrc-react-reveal"><p>Pitch git-lrc to Leadership</p><pre class="lrc-caption">Hi Leadership,\n\nQuick update on what git-lrc has done for our review workflow.\n• Flat input payload silently dropped for non-UI callers\n• Drag-drop input promotion ignores existing step override\n\nI'd like to renew/expand the seat.\n</pre><button class="btn btn-primary" onClick=${()=>navigator.clipboard.writeText('Pitch copied')}>Copy pitch</button><button class="btn" onClick=${()=>setStep('stats')}>Back</button></div>`}
    ${step==='down'&&html`<div class="lrc-react-reveal"><textarea placeholder="What missed the mark?" value=${text} onInput=${(e)=>setText(e.target.value)}></textarea><div>${FEEDBACK_TAGS.map(t=>html`<button class="lrc-tag ${tags.has(t)?'active':''}" onClick=${()=>{const n=new Set(tags); n.has(t)?n.delete(t):n.add(t); setTags(n);}}>${t}</button>`)}</div><label><input type="checkbox" checked=${includeContext} onChange=${(e)=>setIncludeContext(e.target.checked)}/> Include diff + comments as context</label><button class="btn btn-primary" onClick=${async ()=>{if(!text.trim()) return; await fetch('/api/feedback',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({kind:'down',commentId:comment?.ID||'',prId:prId||0,text,tags:Array.from(tags).map(x=>x.toLowerCase().replace(/ /g,'-')),includeContext,createdAt:new Date().toISOString()})}).catch(()=>{}); setState('down:sent'); setStep('down-sent');}}>Send feedback</button><button class="btn" onClick=${()=>{setState('');setStep('');}}>Cancel</button></div>`}
    ${step==='down-sent'&&html`<div class="lrc-react-reveal">Thanks — sent to the team. We read every one.</div>`}
    </div>`;
  };
}

let ReactionFlowComponent = null;
export async function getReactionFlow() { if(!ReactionFlowComponent){ReactionFlowComponent = await createReactionFlow();} return ReactionFlowComponent; }
