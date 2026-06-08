import { motion } from 'framer-motion'

/** Decorative git commit graph: branch curves + shaped nodes, GitHub-style. */
export default function GitGraph() {
  const draw = {
    hidden: { pathLength: 0, opacity: 0 },
    show: (i: number) => ({
      pathLength: 1,
      opacity: 1,
      transition: { pathLength: { duration: 0.9, delay: 0.2 + i * 0.15, ease: [0.65, 0, 0.35, 1] as [number, number, number, number] }, opacity: { duration: 0.2, delay: 0.2 + i * 0.15 } },
    }),
  }
  const pop = {
    hidden: { scale: 0, opacity: 0 },
    show: (i: number) => ({ scale: 1, opacity: 1, transition: { type: 'spring' as const, stiffness: 320, damping: 18, delay: 0.3 + i * 0.12 } }),
  }

  return (
    <div className="flex items-center justify-center">
      <motion.svg
        viewBox="0 0 360 460"
        className="h-auto w-full max-w-[420px]"
        initial="hidden"
        animate="show"
      >
        {/* guide lines */}
        <line x1="96" y1="40" x2="96" y2="420" className="stroke-zinc-200 dark:stroke-white/10" strokeWidth={2} />
        <line x1="200" y1="150" x2="200" y2="330" className="stroke-zinc-200 dark:stroke-white/10" strokeWidth={2} />

        {/* branch curves (animated) */}
        <motion.path custom={0} variants={draw} d="M96 150 C 150 150, 200 160, 200 200" className="fill-none stroke-violet-500" strokeWidth={2.5} strokeLinecap="round" />
        <motion.path custom={1} variants={draw} d="M200 280 C 200 320, 150 320, 96 320" className="fill-none stroke-violet-500" strokeWidth={2.5} strokeLinecap="round" />
        <motion.path custom={2} variants={draw} d="M200 240 C 250 240, 280 230, 280 196" className="fill-none stroke-emerald-500" strokeWidth={2.5} strokeLinecap="round" />

        {/* nodes */}
        <motion.g custom={0} variants={pop}>
          <circle cx="96" cy="70" r="19" className="fill-blue-100 stroke-blue-500 dark:fill-blue-500/20" strokeWidth={2.5} />
          <rect x="88" y="62" width="16" height="16" rx="2" transform="rotate(45 96 70)" className="fill-blue-600" />
        </motion.g>
        <motion.g custom={1} variants={pop}>
          <circle cx="96" cy="150" r="19" className="fill-violet-100 stroke-violet-500 dark:fill-violet-500/20" strokeWidth={2.5} />
          <rect x="89" y="143" width="14" height="14" rx="2" className="fill-violet-600" />
        </motion.g>
        <motion.g custom={2} variants={pop}>
          <circle cx="200" cy="200" r="19" className="fill-emerald-100 stroke-emerald-500 dark:fill-emerald-500/20" strokeWidth={2.5} />
          <polygon points="200,191 208,206 192,206" className="fill-emerald-600" />
        </motion.g>
        <motion.g custom={3} variants={pop}>
          <circle cx="200" cy="280" r="19" className="fill-violet-100 stroke-violet-500 dark:fill-violet-500/20" strokeWidth={2.5} />
          <circle cx="200" cy="280" r="5" className="fill-none stroke-violet-600" strokeWidth={3.5} />
        </motion.g>
        <motion.g custom={4} variants={pop}>
          <circle cx="280" cy="196" r="17" className="fill-emerald-100 stroke-emerald-500 dark:fill-emerald-500/20" strokeWidth={2.5} />
          <rect x="273" y="189" width="14" height="14" rx="2" transform="rotate(45 280 196)" className="fill-emerald-600" />
        </motion.g>
        <motion.g custom={5} variants={pop}>
          <circle cx="96" cy="320" r="19" className="fill-blue-100 stroke-blue-500 dark:fill-blue-500/20" strokeWidth={2.5} />
          <circle cx="96" cy="320" r="6" className="fill-blue-600" />
        </motion.g>
        <motion.g custom={6} variants={pop}>
          <circle cx="96" cy="400" r="19" className="fill-blue-100 stroke-blue-500 dark:fill-blue-500/20" strokeWidth={2.5} />
          <rect x="89" y="393" width="14" height="14" rx="2" className="fill-blue-600" />
        </motion.g>

        {/* labels */}
        <text x="124" y="74" className="fill-zinc-400 font-mono text-[11px] font-semibold">main</text>
        <text x="228" y="204" className="fill-zinc-400 font-mono text-[11px] font-semibold">review</text>
        <text x="124" y="404" className="fill-zinc-400 font-mono text-[11px] font-semibold">shipped</text>
      </motion.svg>
    </div>
  )
}
