import { Star, GitFork, Eye } from 'lucide-react'
import { GithubMark } from './icons'

const topics = ['ai', 'code-review', 'git', 'cli', 'golang', 'devtools']

/** A faithful GitHub repository card. */
export default function GithubRepoCard() {
  return (
    <div className="w-full max-w-md rounded-xl border border-zinc-200 bg-white p-5 shadow-xl shadow-zinc-900/5 dark:border-white/10 dark:bg-zinc-900">
      {/* header */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <GithubMark size={20} className="shrink-0 text-zinc-900 dark:text-white" />
          <span className="truncate text-[15px] font-semibold">
            <span className="text-zinc-500 dark:text-zinc-400">HexmosTech / </span>
            <a href="https://github.com/HexmosTech/git-lrc" target="_blank" rel="noreferrer" className="text-brand-600 hover:underline dark:text-brand-400">git-lrc</a>
          </span>
        </div>
        <span className="shrink-0 rounded-full border border-zinc-300 px-2.5 py-0.5 text-[11px] font-medium text-zinc-500 dark:border-white/15 dark:text-zinc-400">
          Public
        </span>
      </div>

      {/* description */}
      <p className="mt-3 text-[13.5px] leading-relaxed text-zinc-600 dark:text-zinc-300">
        AI-powered code review in your <code className="font-mono text-[0.85em]">git commit</code> flow — free, micro reviews that run on commit.
      </p>

      {/* topics */}
      <div className="mt-3.5 flex flex-wrap gap-2">
        {topics.map((t) => (
          <span key={t} className="rounded-full bg-brand-600/10 px-2.5 py-1 text-[11.5px] font-medium text-brand-600 dark:bg-brand-400/10 dark:text-brand-400">
            {t}
          </span>
        ))}
      </div>

      {/* meta row */}
      <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-[12.5px] text-zinc-500 dark:text-zinc-400">
        <span className="inline-flex items-center gap-1.5">
          <span className="h-3 w-3 rounded-full bg-[#00ADD8]" /> Go
        </span>
        <span className="inline-flex items-center gap-1.5"><Star size={14} /> 1k+</span>
        <span className="inline-flex items-center gap-1.5"><GitFork size={14} /> 150+</span>
        <span className="inline-flex items-center gap-1.5"><Eye size={14} /> Watch</span>
        <span className="ml-auto">Updated today</span>
      </div>

      {/* star button */}
      <a
        href="https://github.com/HexmosTech/git-lrc"
        target="_blank"
        rel="noreferrer"
        className="mt-4 flex items-center justify-center gap-2 rounded-lg border border-zinc-300 bg-zinc-50 py-2.5 text-[13px] font-semibold text-zinc-800 transition hover:bg-zinc-100 dark:border-white/15 dark:bg-white/5 dark:text-zinc-100 dark:hover:bg-white/10"
      >
        <Star size={15} className="text-amber-500" /> Star on GitHub
      </a>
    </div>
  )
}
