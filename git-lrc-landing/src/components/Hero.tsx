import { motion } from 'framer-motion'
import { ArrowRight, Check } from 'lucide-react'
import { GithubMark } from './icons'
import GitGraph from './GitGraph'

const checks = ['60-second setup', 'Runs locally by default', 'No cost']

export default function Hero() {
  return (
    <header id="top" className="mx-auto grid max-w-6xl items-center gap-14 px-6 pb-20 pt-20 md:grid-cols-[1.05fr_0.95fr] md:pt-24">
      <div>
        <motion.span
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4 }}
          className="mb-6 inline-flex items-center gap-2 rounded-full border border-brand-600/20 bg-brand-600/[0.07] px-3 py-1.5 text-[12.5px] font-semibold text-brand-600 dark:text-brand-400"
        >
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.2)]" />
          Free · runs on every commit
        </motion.span>

        <motion.h1
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.05 }}
          className="text-[clamp(40px,5.4vw,66px)] font-extrabold leading-[1.02] tracking-[-0.035em] text-zinc-900 dark:text-white"
        >
          AI code review,
          <br />
          <span className="bg-gradient-to-br from-brand-500 to-brand-700 bg-clip-text text-transparent">
            before it lands.
          </span>
        </motion.h1>

        <motion.p
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.12 }}
          className="mt-6 max-w-[30em] text-[19px] text-zinc-500 dark:text-zinc-400"
        >
          git-lrc hooks into <code className="rounded bg-zinc-100 px-1.5 py-0.5 font-mono text-[0.85em] text-zinc-700 dark:bg-white/10 dark:text-zinc-200">git commit</code> and
          reviews every diff the moment you write it — catching silently removed logic, behavior changes and bugs before they reach production.
        </motion.p>

        <motion.div
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.18 }}
          className="mt-8 flex flex-wrap items-center gap-3.5"
        >
          <a
            href="https://hexmos.com/livereview/git-lrc/"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-2 rounded-full bg-gradient-to-br from-brand-500 to-brand-700 px-6 py-3.5 text-[15px] font-semibold text-white shadow-xl shadow-brand-600/30 transition hover:-translate-y-0.5"
          >
            Get started <ArrowRight size={17} />
          </a>
          <a
            href="https://github.com/HexmosTech/git-lrc"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-white px-6 py-3.5 text-[15px] font-semibold text-zinc-900 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md dark:border-white/10 dark:bg-zinc-900 dark:text-white"
          >
            <GithubMark size={17} /> Star on GitHub
          </a>
        </motion.div>

        <div className="mt-7 flex flex-wrap items-center gap-5 text-[13px] text-zinc-400 dark:text-zinc-500">
          {checks.map((c) => (
            <span key={c} className="inline-flex items-center gap-1.5">
              <Check size={15} className="text-emerald-500" /> {c}
            </span>
          ))}
        </div>
      </div>

      <motion.div
        initial={{ opacity: 0, scale: 0.96 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ duration: 0.6, delay: 0.1, ease: [0.22, 1, 0.36, 1] }}
      >
        <GitGraph />
      </motion.div>
    </header>
  )
}
