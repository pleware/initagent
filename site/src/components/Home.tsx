import { Hero } from "./Hero";
import { HowItWorks } from "./HowItWorks";
import { Capabilities } from "./Capabilities";
import { NextPages } from "./NextPages";

export function Home() {
  return (
    <main>
      <Hero />
      <HowItWorks />
      <Capabilities />
      <NextPages />
    </main>
  );
}
