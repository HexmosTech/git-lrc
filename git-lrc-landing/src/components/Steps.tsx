import Reveal from './Reveal'
import SectionHead from './SectionHead'

const steps = [
  { n: 1, title: 'Install', body: 'Build locally and install the git hook in one line.', cmd: 'lrc hooks install' },
  { n: 2, title: 'Commit', body: 'Run git like always — git-lrc reviews the diff before it lands.', cmd: 'git commit -m "…"' },
  { n: 3, title: 'Ship', body: 'Read the findings, fix what matters, push with confidence.', cmd: 'git push' },
]

export default function Steps() {
  return (
    <section id="how" className="mx-auto max-w-6xl px-6 py-20">
      <SectionHead kicker="How it works" title="Set up once. Review every commit." sub="From install to your first review in under a minute." />
      <div className="relative grid gap-5 md:grid-cols-3">
        {/* connector line (desktop) */}
        <div className="pointer-events-none absolute left-[16%] right-[16%] top-[34px] hidden h-px bg-gradient-to-r from-transparent via-zinc-200 to-transparent dark:via-white/10 md:block" />
        {steps.map((s, i) => (
          <Reveal key={s.n} delay={i * 0.1}>
            <div className="relative flex h-full flex-col rounded-2xl border border-zinc-200 bg-white p-7 shadow-sm dark:border-white/10 dark:bg-zinc-900">
              <span className="relative z-10 mb-5 grid h-10 w-10 place-items-center rounded-full bg-gradient-to-br from-brand-500 to-brand-700 text-[15px] font-extrabold text-white shadow-lg shadow-brand-600/30 ring-4 ring-white dark:ring-zinc-900">
                {s.n}
              </span>
              <h3 className="mb-1.5 text-[16px] font-bold tracking-tight text-zinc-900 dark:text-white">{s.title}</h3>
              <p className="mb-5 text-[13.5px] leading-relaxed text-zinc-500 dark:text-zinc-400">{s.body}</p>
              {/* terminal command chip */}
              <div className="mt-auto flex items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2.5 dark:border-white/10 dark:bg-zinc-950/60">
                <span className="select-none font-mono text-[12px] text-emerald-500">$</span>
                <code className="font-mono text-[12.5px] text-zinc-700 dark:text-zinc-200">{s.cmd}</code>
              </div>
            </div>
          </Reveal>
        ))}
      </div>
    </section>
  )
}
