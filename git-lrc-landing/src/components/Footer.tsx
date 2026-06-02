const links = [
  { label: 'GitHub', href: 'https://github.com/HexmosTech/git-lrc' },
  { label: 'Docs', href: 'https://hexmos.com/livereview/git-lrc/' },
  { label: 'Discord', href: 'https://discord.gg/sGdnKwB3qq' },
]

export default function Footer() {
  return (
    <footer className="mt-6 border-t border-zinc-200 py-10 dark:border-white/10">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-5 px-6 text-[13px] text-zinc-400">
        <a href="#top" className="flex items-center gap-2 text-[15px] font-extrabold text-zinc-900 dark:text-white">
          <span className="grid h-6 w-6 place-items-center rounded-md bg-gradient-to-br from-brand-500 to-brand-700 text-white">
            <span className="block h-2 w-2 rounded-full ring-2 ring-white/90" />
          </span>
          git-lrc
        </a>
        {links.map((l) => (
          <a key={l.label} href={l.href} target="_blank" rel="noreferrer" className="text-zinc-500 transition-colors hover:text-zinc-900 dark:hover:text-white">
            {l.label}
          </a>
        ))}
        <span className="ml-auto">© Hexmos · LiveReview</span>
      </div>
    </footer>
  )
}
