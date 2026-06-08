import { Check, CircleDot, Circle } from 'lucide-react'
import Reveal from './Reveal'
import SectionHead from './SectionHead'

type Card = {
  title: string
  updated: string
  desc: string
  done: number
  progress: number
  todo: number
  pct: number
}

const cards: Card[] = [
  { title: 'feat/auth-refactor', updated: 'Updated 2 days ago', desc: 'Session handling + token validation.', done: 6, progress: 0, todo: 1, pct: 86 },
  { title: 'fix/parser-edge-cases', updated: 'Updated 5 hours ago', desc: 'Realign downstream parser expectations.', done: 2, progress: 1, todo: 1, pct: 50 },
  { title: 'chore/config-telemetry', updated: 'Updated 1 day ago', desc: 'Disable telemetry leak in generated config.', done: 0, progress: 1, todo: 2, pct: 12 },
  { title: 'release/v0.4.x', updated: 'Updated 2 months ago', desc: 'Cut and verify the next release.', done: 20, progress: 0, todo: 0, pct: 100 },
]

function Stat({ icon, n, label, color }: { icon: React.ReactNode; n: number; label: string; color: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-[12.5px] text-zinc-500 dark:text-zinc-400">
      <span className={color}>{icon}</span>
      <span className="font-semibold text-zinc-700 dark:text-zinc-200">{n}</span> {label}
    </span>
  )
}

export default function ProjectBoard() {
  return (
    <section id="board" className="mx-auto max-w-6xl px-6 py-16">
      <SectionHead
        kicker="Stay on top of it"
        title="Every review, tracked like a project"
        sub="git-lrc groups findings by branch so you always know what's resolved, what's in progress and what still needs attention."
      />
      <div className="grid gap-4 md:grid-cols-2">
        {cards.map((c, i) => (
          <Reveal key={c.title} delay={(i % 2) * 0.08}>
            <div className="h-full rounded-2xl border border-zinc-200 bg-white p-5 shadow-sm transition hover:-translate-y-1 hover:shadow-lg dark:border-white/10 dark:bg-zinc-900">
              <div className="flex items-baseline justify-between gap-3">
                <h3 className="font-mono text-[14px] font-bold text-zinc-900 dark:text-white">{c.title}</h3>
                <span className="shrink-0 text-[11.5px] text-zinc-400">{c.updated}</span>
              </div>
              <p className="mt-1.5 text-[13px] text-zinc-500 dark:text-zinc-400">{c.desc}</p>

              {/* progress bar */}
              <div className="mt-4 h-1.5 w-full overflow-hidden rounded-full bg-zinc-100 dark:bg-white/10">
                <div
                  className="h-full rounded-full bg-gradient-to-r from-emerald-400 to-emerald-600"
                  style={{ width: `${c.pct}%` }}
                />
              </div>

              {/* status row */}
              <div className="mt-3.5 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-zinc-100 pt-3.5 dark:border-white/5">
                <Stat icon={<Check size={15} strokeWidth={3} />} n={c.done} label="resolved" color="text-emerald-500" />
                <Stat icon={<CircleDot size={15} />} n={c.progress} label="in review" color="text-amber-500" />
                <Stat icon={<Circle size={15} />} n={c.todo} label="open" color="text-zinc-400" />
              </div>
            </div>
          </Reveal>
        ))}
      </div>
    </section>
  )
}
