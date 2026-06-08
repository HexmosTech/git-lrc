import { ClipboardCheck, Search, ShieldCheck, AlertOctagon } from 'lucide-react'
import Reveal from './Reveal'
import SectionHead from './SectionHead'

function Dots() {
  return (
    <div className="flex items-center gap-1.5">
      <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
      <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
      <span className="h-3 w-3 rounded-full bg-[#28c840]" />
    </div>
  )
}

const stats = [
  { icon: ClipboardCheck, label: 'Reviews', value: '1,284', tone: 'text-brand-600 bg-brand-600/10' },
  { icon: Search, label: 'Issues', value: '3,907', tone: 'text-amber-600 bg-amber-500/15' },
  { icon: ShieldCheck, label: 'Bugs caught', value: '612', tone: 'text-emerald-600 bg-emerald-500/15' },
  { icon: AlertOctagon, label: 'Critical', value: '48', tone: 'text-red-600 bg-red-500/12' },
]

const rows = [
  { file: 'src/auth/session.go', sev: 'Critical', cls: 'bg-red-500/10 text-red-600 ring-red-500/30', ago: '2m ago' },
  { file: 'api/handlers.go', sev: 'Warning', cls: 'bg-amber-400/15 text-amber-700 ring-amber-500/30', ago: '14m ago' },
  { file: 'README.md', sev: 'Info', cls: 'bg-brand-500/10 text-brand-600 ring-brand-500/30', ago: '1h ago' },
]

export default function Showcase() {
  return (
    <section id="showcase" className="mx-auto max-w-6xl px-6 py-16">
      <SectionHead kicker="See it in action" title="From commit to caught — in seconds" sub="git-lrc reviews the diff the moment you commit, then opens a clean dashboard with every finding." />

      <div className="grid items-stretch gap-5 lg:grid-cols-[1.25fr_0.75fr]">
        {/* Terminal */}
        <Reveal className="h-full">
          <div className="flex h-full flex-col overflow-hidden rounded-2xl border border-zinc-200 bg-white shadow-2xl shadow-zinc-900/10 dark:border-white/10 dark:bg-zinc-900">
            <div className="flex items-center gap-2 border-b border-zinc-200 bg-zinc-50 px-4 py-3 dark:border-white/10 dark:bg-zinc-950/50">
              <Dots />
              <span className="ml-2 font-mono text-xs text-zinc-400">zsh — git-lrc</span>
            </div>
            <div className="flex flex-1 flex-col space-y-1 p-5 font-mono text-[12.5px]">
              <div className="text-zinc-400">$ git commit -m "refactor auth session"</div>
              <div className="text-zinc-700 dark:text-zinc-200">→ git-lrc reviewing staged diff…</div>
              <div className="text-zinc-400">&nbsp;&nbsp;src/auth/session.go</div>
              <div className="rounded bg-emerald-500/10 px-2 py-0.5 text-emerald-600 dark:text-emerald-400">+ &nbsp;&nbsp;if token == "" {'{'} return nil {'}'}</div>
              <div className="mt-3.5 rounded-xl border border-zinc-200 bg-white p-3.5 font-sans shadow-sm dark:border-white/10 dark:bg-zinc-950/60">
                <div className="mb-2 flex items-center gap-2">
                  <span className="rounded-full bg-red-500/12 px-2.5 py-1 text-[10px] font-extrabold uppercase tracking-wide text-red-600 ring-1 ring-red-500/30">Critical</span>
                  <span className="text-[13px] font-bold text-zinc-900 dark:text-white">Logic</span>
                  <span className="text-[11px] text-zinc-400">caught in 0.8s</span>
                </div>
                <p className="text-[13px] leading-relaxed text-zinc-600 dark:text-zinc-300">
                  Early-return on an empty token skips <b>validateSession</b> — unauthenticated requests would pass. Gate this before committing.
                </p>
              </div>
              <div className="mt-auto pt-2.5 text-zinc-400">&nbsp;&nbsp;1 critical · 2 warnings · review at localhost:8000</div>
            </div>
          </div>
        </Reveal>

        {/* Dashboard */}
        <Reveal delay={0.1} className="h-full">
          <div className="flex h-full flex-col overflow-hidden rounded-2xl border border-zinc-200 bg-white shadow-2xl shadow-zinc-900/10 dark:border-white/10 dark:bg-zinc-900">
            <div className="flex items-center gap-2 border-b border-zinc-200 bg-zinc-50 px-4 py-3 dark:border-white/10 dark:bg-zinc-950/50">
              <Dots />
              <span className="ml-2 truncate rounded-full bg-white px-3 py-1 font-mono text-[11px] text-zinc-400 ring-1 ring-zinc-200 dark:bg-zinc-950 dark:ring-white/10">
                localhost:8000 · dashboard
              </span>
            </div>
            <div className="flex flex-1 flex-col p-5">
              <div className="mb-4 flex items-baseline justify-between">
                <h4 className="text-[15px] font-extrabold tracking-tight text-zinc-900 dark:text-white">Review impact</h4>
                <span className="text-xs text-zinc-400">last 30 days</span>
              </div>
              <div className="mb-4 grid grid-cols-2 gap-2.5">
                {stats.map((s) => (
                  <div key={s.label} className="rounded-xl border border-zinc-200 bg-white p-3 shadow-sm dark:border-white/10 dark:bg-zinc-950/50">
                    <div className="mb-2 flex items-center gap-2">
                      <span className={`grid h-6 w-6 place-items-center rounded-md ${s.tone}`}><s.icon size={14} /></span>
                      <span className="text-[11px] font-semibold text-zinc-500 dark:text-zinc-400">{s.label}</span>
                    </div>
                    <div className="text-[22px] font-extrabold tracking-tight tabular-nums text-zinc-900 dark:text-white">{s.value}</div>
                  </div>
                ))}
              </div>
              <div className="space-y-2">
                {rows.map((r) => (
                  <div key={r.file} className="flex items-center gap-3 rounded-xl border border-zinc-200 bg-white px-3.5 py-2.5 shadow-sm dark:border-white/10 dark:bg-zinc-950/50">
                    <span className="font-mono text-[13px] font-semibold text-zinc-800 dark:text-zinc-100">{r.file}</span>
                    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-extrabold uppercase tracking-wide ring-1 ${r.cls}`}>{r.sev}</span>
                    <span className="ml-auto text-xs text-zinc-400">{r.ago}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </Reveal>
      </div>
    </section>
  )
}
