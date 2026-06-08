import { BookOpen, Rocket, Plug, MessagesSquare, ArrowUpRight } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import Reveal from './Reveal'
import SectionHead from './SectionHead'

type Doc = {
  icon: LucideIcon
  title: string
  category: string
  body: string
  tags: string[]
  dot: string
  meta: string
  href: string
  tone: string
}

const docs: Doc[] = [
  {
    icon: Rocket,
    title: 'Quickstart',
    category: 'Guide',
    body: 'Install the git hook and run your first review in 60 seconds.',
    tags: ['install', 'hooks', 'cli'],
    dot: 'bg-brand-500',
    meta: '2 min read',
    href: 'https://hexmos.com/livereview/git-lrc/',
    tone: 'text-brand-600 bg-brand-600/10',
  },
  {
    icon: BookOpen,
    title: 'Documentation',
    category: 'Reference',
    body: 'Configuration, hooks, severity levels and the local review UI.',
    tags: ['config', 'severity', 'ui'],
    dot: 'bg-violet-500',
    meta: 'docs',
    href: 'https://hexmos.com/livereview/git-lrc/',
    tone: 'text-violet-600 bg-violet-500/12',
  },
  {
    icon: Plug,
    title: 'AI connectors',
    category: 'Setup',
    body: 'Add Gemini, Claude, OpenAI or DeepSeek and set the priority order.',
    tags: ['gemini', 'claude', 'openai'],
    dot: 'bg-amber-500',
    meta: 'guide',
    href: '/static/ui-connectors.html#/connectors',
    tone: 'text-amber-600 bg-amber-500/15',
  },
  {
    icon: MessagesSquare,
    title: 'Community',
    category: 'Support',
    body: 'Ask questions and share ideas on Discussions and Discord.',
    tags: ['discussions', 'discord'],
    dot: 'bg-emerald-500',
    meta: 'join',
    href: 'https://github.com/HexmosTech/git-lrc/discussions',
    tone: 'text-emerald-600 bg-emerald-500/12',
  },
]

export default function Docs() {
  return (
    <section id="docs" className="mx-auto max-w-6xl px-6 py-20">
      <SectionHead kicker="Resources" title="Docs & everything to get going" sub="Short, practical guides — from your first commit review to wiring up multiple AI providers." />
      <div className="grid gap-4 sm:grid-cols-2">
        {docs.map((d, i) => (
          <Reveal key={d.title} delay={(i % 2) * 0.07}>
            <a
              href={d.href}
              target={d.href.startsWith('http') ? '_blank' : undefined}
              rel="noreferrer"
              className="group block h-full rounded-xl border border-zinc-200 bg-white p-5 shadow-sm transition duration-300 hover:-translate-y-0.5 hover:border-zinc-300 hover:shadow-lg hover:shadow-zinc-900/5 dark:border-white/10 dark:bg-zinc-900 dark:hover:border-white/20"
            >
              {/* header — GitHub repo-card style */}
              <div className="flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2.5">
                  <span className={`grid h-7 w-7 shrink-0 place-items-center rounded-lg ${d.tone}`}>
                    <d.icon size={15} strokeWidth={2} />
                  </span>
                  <span className="truncate text-[14px] font-semibold text-brand-600 group-hover:underline dark:text-brand-400">
                    {d.title}
                  </span>
                </div>
                <span className="shrink-0 rounded-full border border-zinc-300 px-2 py-0.5 text-[10.5px] font-medium text-zinc-500 dark:border-white/15 dark:text-zinc-400">
                  {d.category}
                </span>
              </div>

              {/* description — small */}
              <p className="mt-2.5 text-[12.5px] leading-relaxed text-zinc-500 dark:text-zinc-400">{d.body}</p>

              {/* topic tags — like repo card */}
              <div className="mt-3 flex flex-wrap gap-1.5">
                {d.tags.map((t) => (
                  <span key={t} className="rounded-full bg-zinc-100 px-2 py-0.5 font-mono text-[10.5px] text-zinc-500 dark:bg-white/5 dark:text-zinc-400">
                    {t}
                  </span>
                ))}
              </div>

              {/* meta row — language-dot + meta + arrow */}
              <div className="mt-4 flex items-center gap-2 border-t border-zinc-100 pt-3 text-[11.5px] text-zinc-400 dark:border-white/5 dark:text-zinc-500">
                <span className={`h-2.5 w-2.5 rounded-full ${d.dot}`} />
                <span className="font-mono">{d.meta}</span>
                <ArrowUpRight size={14} className="ml-auto text-zinc-300 transition group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-brand-500 dark:text-zinc-600" />
              </div>
            </a>
          </Reveal>
        ))}
      </div>
    </section>
  )
}
