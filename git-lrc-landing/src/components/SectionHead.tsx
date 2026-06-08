import Reveal from './Reveal'

export default function SectionHead({ kicker, title, sub }: { kicker: string; title: string; sub: string }) {
  return (
    <Reveal className="mx-auto mb-12 max-w-2xl text-center">
      <div className="mb-3 text-[13px] font-bold uppercase tracking-[0.08em] text-brand-600 dark:text-brand-400">{kicker}</div>
      <h2 className="text-[clamp(28px,3.6vw,40px)] font-extrabold tracking-[-0.03em] text-zinc-900 dark:text-white">{title}</h2>
      <p className="mt-3.5 text-[17px] text-zinc-500 dark:text-zinc-400">{sub}</p>
    </Reveal>
  )
}
