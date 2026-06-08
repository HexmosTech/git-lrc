import Reveal from './Reveal'

export default function CTA() {
  return (
    <section className="mx-auto max-w-6xl px-6 py-16">
      <Reveal>
        <div className="relative overflow-hidden rounded-3xl border border-zinc-200 bg-white px-8 py-14 text-center shadow-xl dark:border-white/10 dark:bg-zinc-900">
          <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(600px_220px_at_50%_-20%,rgba(10,111,209,0.12),transparent_70%)]" />
          <h2 className="relative text-[clamp(26px,3.4vw,38px)] font-extrabold tracking-[-0.03em] text-zinc-900 dark:text-white">
            Catch the bug before your users do.
          </h2>
          <p className="relative mx-auto mt-3.5 max-w-xl text-[17px] text-zinc-500 dark:text-zinc-400">
            Free, unlimited AI code reviews that run on every commit. 60-second setup.
          </p>
          <div className="relative mt-7 flex flex-wrap justify-center gap-3.5">
            <a
              href="https://hexmos.com/livereview/git-lrc/"
              target="_blank"
              rel="noreferrer"
              className="rounded-full bg-gradient-to-br from-brand-500 to-brand-700 px-6 py-3.5 text-[15px] font-semibold text-white shadow-xl shadow-brand-600/30 transition hover:-translate-y-0.5"
            >
              Get started
            </a>
            <a
              href="/static/ui-connectors.html#/home"
              className="rounded-full border border-zinc-200 bg-white px-6 py-3.5 text-[15px] font-semibold text-zinc-900 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md dark:border-white/10 dark:bg-zinc-950 dark:text-white"
            >
              Open dashboard
            </a>
          </div>
        </div>
      </Reveal>
    </section>
  )
}
