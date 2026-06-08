import { Moon, Sun } from 'lucide-react'
import { GithubMark } from './icons'
import { useTheme } from '../lib/useTheme'

const links = [
  { label: 'Features', href: '#features' },
  { label: 'How it works', href: '#how' },
  { label: 'Showcase', href: '#showcase' },
  { label: 'Docs', href: '#docs' },
]

export default function Nav() {
  const { theme, toggle } = useTheme()
  return (
    <nav className="sticky top-0 z-50 border-b border-zinc-200/70 bg-white/70 backdrop-blur-xl backdrop-saturate-150 dark:border-white/10 dark:bg-zinc-950/60">
      <div className="mx-auto flex max-w-6xl items-center gap-7 px-6 py-3.5">
        <a href="#top" className="flex items-center gap-2.5 text-[18px] font-extrabold tracking-tight text-zinc-900 dark:text-white">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-gradient-to-br from-brand-500 to-brand-700 text-white shadow-sm">
            <span className="block h-2.5 w-2.5 rounded-full ring-2 ring-white/90" />
          </span>
          git-lrc
        </a>
        <div className="ml-2 hidden items-center gap-6 md:flex">
          {links.map((l) => (
            <a
              key={l.label}
              href={l.href}
              className="text-sm font-medium text-zinc-500 transition-colors hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-white"
            >
              {l.label}
            </a>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-2.5">
          <button
            onClick={toggle}
            aria-label="Toggle theme"
            className="grid h-9 w-9 place-items-center rounded-full border border-zinc-200 bg-white text-zinc-600 transition hover:-translate-y-0.5 hover:text-zinc-900 hover:shadow-md dark:border-white/10 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:text-white"
          >
            {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
          </button>
          <a
            href="https://github.com/HexmosTech/git-lrc"
            target="_blank"
            rel="noreferrer"
            className="hidden items-center gap-2 rounded-full px-3 py-2 text-sm font-semibold text-zinc-700 transition hover:text-zinc-900 dark:text-zinc-300 dark:hover:text-white sm:flex"
          >
            <GithubMark size={17} /> GitHub
          </a>
          <a
            href="/static/ui-connectors.html#/home"
            className="rounded-full bg-gradient-to-br from-brand-500 to-brand-700 px-5 py-2 text-sm font-semibold text-white shadow-lg shadow-brand-600/25 transition hover:-translate-y-0.5"
          >
            Open dashboard
          </a>
        </div>
      </div>
    </nav>
  )
}
