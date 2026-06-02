import Nav from './components/Nav'
import Hero from './components/Hero'
import Showcase from './components/Showcase'
import Features from './components/Features'
import ProjectBoard from './components/ProjectBoard'
import OpenSource from './components/OpenSource'
import Steps from './components/Steps'
import Docs from './components/Docs'
import FAQ from './components/FAQ'
import CTA from './components/CTA'
import Footer from './components/Footer'

export default function App() {
  return (
    <div className="min-h-screen bg-white text-zinc-700 dark:bg-zinc-950 dark:text-zinc-300">
      {/* ambient gradient */}
      <div className="pointer-events-none fixed inset-0 -z-10 bg-[radial-gradient(1100px_620px_at_100%_-8%,rgba(99,91,255,0.07),transparent_55%),radial-gradient(1000px_560px_at_-8%_2%,rgba(10,111,209,0.07),transparent_55%)] dark:bg-[radial-gradient(1100px_620px_at_100%_-8%,rgba(99,91,255,0.10),transparent_55%),radial-gradient(1000px_560px_at_-8%_2%,rgba(10,111,209,0.12),transparent_55%)]" />
      <Nav />
      <main>
        <Hero />
        <Showcase />
        <Features />
        <ProjectBoard />
        <OpenSource />
        <Steps />
        <Docs />
        <FAQ />
        <CTA />
      </main>
      <Footer />
    </div>
  )
}
