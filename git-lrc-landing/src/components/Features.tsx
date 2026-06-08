import { GitCommitHorizontal, Gift, Lock, Cpu, ShieldAlert, MessageSquareCode } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import Reveal from './Reveal'
import SectionHead from './SectionHead'

type Feature = { icon: LucideIcon; title: string; body: string; tag: string; tone: string }

const features: Feature[] = [
  { icon: GitCommitHorizontal, title: 'Runs on commit', body: 'Hooks into git commit and reviews the staged diff automatically — no extra step, no context switch.', tag: 'git hook', tone: 'text-brand-600 bg-brand-600/10' },
  { icon: Gift, title: 'Completely free', body: 'Unlimited AI reviews at no cost. Set it up once and review every commit, forever.', tag: '$0 · unlimited', tone: 'text-emerald-600 bg-emerald-500/12' },
  { icon: Lock, title: 'Runs locally', body: 'A local CLI by default — your code stays on your machine unless you submit a review.', tag: 'local-first', tone: 'text-violet-600 bg-violet-500/12' },
  { icon: Cpu, title: 'Any AI provider', body: 'Bring your own connector — Gemini, Claude, OpenAI, DeepSeek and more, prioritized your way.', tag: 'BYO model', tone: 'text-amber-600 bg-amber-500/15' },
  { icon: ShieldAlert, title: 'Severity-ranked', body: 'Findings graded Critical, Error, Warning and Info, so you fix what matters before you ship.', tag: '4 levels', tone: 'text-red-600 bg-red-500/12' },
  { icon: MessageSquareCode, title: 'Beautiful review UI', body: 'A clean local viewer with inline diffs, comments and one-click hand-off to your coding agent.', tag: 'web UI', tone: 'text-brand-600 bg-brand-600/10' },
]

export default function Features() {
  return (
    <section id="features" className="mx-auto max-w-6xl px-6 py-20">
      <SectionHead kicker="Why git-lrc" title="Reviews that catch what you miss" sub="AI agents write code fast — and silently remove logic, change behavior and introduce bugs. git-lrc reviews every diff before it lands." />
      <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        {features.map((f, i) => (
          <Reveal key={f.title} delay={(i % 3) * 0.07}>
            <div className="group flex h-full flex-col rounded-2xl border border-zinc-200 bg-white p-7 shadow-sm transition duration-300 hover:-translate-y-1 hover:border-zinc-300 hover:shadow-xl hover:shadow-zinc-900/5 dark:border-white/10 dark:bg-zinc-900 dark:hover:border-white/20">
              <span className={`mb-5 grid h-12 w-12 place-items-center rounded-xl ${f.tone} transition group-hover:scale-105`}>
                <f.icon size={22} strokeWidth={2} />
              </span>
              <h3 className="mb-2 text-[16px] font-bold tracking-tight text-zinc-900 dark:text-white">{f.title}</h3>
              <p className="text-[13.5px] leading-relaxed text-zinc-500 dark:text-zinc-400">{f.body}</p>
              <div className="mt-5 flex items-center justify-between border-t border-zinc-100 pt-4 dark:border-white/5">
                <span className="rounded-md bg-zinc-100 px-2 py-1 font-mono text-[11px] font-medium text-zinc-500 dark:bg-white/5 dark:text-zinc-400">
                  {f.tag}
                </span>
                <span className="text-zinc-300 transition group-hover:translate-x-0.5 group-hover:text-brand-500 dark:text-zinc-600">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><line x1="5" y1="12" x2="19" y2="12" /><polyline points="12 5 19 12 12 19" /></svg>
                </span>
              </div>
            </div>
          </Reveal>
        ))}
      </div>
    </section>
  )
}
