import { useState } from 'react'
import { Plus, Minus } from 'lucide-react'
import Reveal from './Reveal'
import SectionHead from './SectionHead'

const faqs = [
  {
    q: 'How does git-lrc review my code?',
    a: 'It installs a git hook that runs on commit, sends the staged diff to your configured AI provider, and surfaces findings — graded by severity — in a local review UI before the commit lands.',
  },
  {
    q: 'Is my code sent anywhere?',
    a: 'git-lrc runs locally by default. Your code only leaves your machine when you explicitly submit a review to your chosen AI provider. There is no required cloud middleman.',
  },
  {
    q: 'Which AI providers are supported?',
    a: 'Bring your own connector — Gemini, Claude, OpenAI, DeepSeek and more. You can add multiple providers and set the exact priority order git-lrc uses.',
  },
  {
    q: 'Is it really free?',
    a: 'Yes. git-lrc itself is free and open source with unlimited reviews. You only pay for your own AI provider usage if your connector requires an API key.',
  },
  {
    q: 'Does it slow down my commits?',
    a: 'Reviews run in seconds and surface asynchronously. You stay in your normal git flow — read the findings, fix what matters, and ship.',
  },
]

function Item({ q, a }: { q: string; a: string }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="rounded-2xl border border-zinc-200 bg-white px-5 shadow-sm dark:border-white/10 dark:bg-zinc-900">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center justify-between gap-4 py-5 text-left"
      >
        <span className="text-[15.5px] font-semibold text-zinc-900 dark:text-white">{q}</span>
        <span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-zinc-100 text-zinc-600 dark:bg-white/10 dark:text-zinc-300">
          {open ? <Minus size={15} /> : <Plus size={15} />}
        </span>
      </button>
      <div
        className="grid transition-all duration-300 ease-out"
        style={{ gridTemplateRows: open ? '1fr' : '0fr' }}
      >
        <div className="overflow-hidden">
          <p className="pb-5 text-[14.5px] leading-relaxed text-zinc-500 dark:text-zinc-400">{a}</p>
        </div>
      </div>
    </div>
  )
}

export default function FAQ() {
  return (
    <section id="faq" className="mx-auto max-w-3xl px-6 py-16">
      <SectionHead kicker="FAQ" title="Questions, answered" sub="Everything you need to know before your first review." />
      <div className="space-y-3">
        {faqs.map((f) => (
          <Reveal key={f.q}>
            <Item {...f} />
          </Reveal>
        ))}
      </div>
    </section>
  )
}
