import { Check } from 'lucide-react'
import { GithubMark } from './icons'
import GithubRepoCard from './GithubRepoCard'
import Reveal from './Reveal'

const points = [
  'MIT-friendly, fully open source — read every line that touches your code.',
  'Active community on GitHub Discussions and Discord.',
  'Runs locally by default; nothing leaves your machine until you submit a review.',
]

export default function OpenSource() {
  return (
    <section id="open-source" className="mx-auto max-w-6xl px-6 py-16">
      <div className="grid items-center gap-12 md:grid-cols-2">
        <Reveal>
          <div>
            <div className="mb-3 text-[13px] font-bold uppercase tracking-[0.08em] text-brand-600 dark:text-brand-400">Open source</div>
            <h2 className="text-[clamp(26px,3.4vw,38px)] font-extrabold tracking-[-0.03em] text-zinc-900 dark:text-white">
              Built in the open, on GitHub
            </h2>
            <p className="mt-3.5 text-[16px] leading-relaxed text-zinc-500 dark:text-zinc-400">
              git-lrc is a transparent, community-driven tool. Inspect the source, open an issue, or
              ship a pull request — the project is designed to be auditable and easy to trust.
            </p>
            <ul className="mt-6 space-y-3">
              {points.map((p) => (
                <li key={p} className="flex items-start gap-3 text-[14.5px] text-zinc-600 dark:text-zinc-300">
                  <span className="mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">
                    <Check size={13} strokeWidth={3} />
                  </span>
                  {p}
                </li>
              ))}
            </ul>
            <div className="mt-7 flex flex-wrap gap-3.5">
              <a
                href="https://github.com/HexmosTech/git-lrc"
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-2 rounded-full bg-zinc-900 px-5 py-3 text-[14px] font-semibold text-white transition hover:-translate-y-0.5 dark:bg-white dark:text-zinc-900"
              >
                <GithubMark size={16} /> View on GitHub
              </a>
              <a
                href="https://github.com/HexmosTech/git-lrc/discussions"
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-white px-5 py-3 text-[14px] font-semibold text-zinc-900 transition hover:-translate-y-0.5 dark:border-white/10 dark:bg-zinc-900 dark:text-white"
              >
                Join the discussion
              </a>
            </div>
          </div>
        </Reveal>

        <Reveal delay={0.1} className="flex justify-center md:justify-end">
          <GithubRepoCard />
        </Reveal>
      </div>
    </section>
  )
}
