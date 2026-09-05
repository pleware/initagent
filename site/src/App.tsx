import { useEffect, type ReactNode } from "react";
import { Nav } from "./components/Nav";
import { Footer } from "./components/Footer";
import { Home } from "./components/Home";
import { Plans } from "./components/Plans";
import { Developers } from "./components/Developers";
import { Hardware } from "./components/Hardware";
import { NotFound } from "./components/NotFound";
import { currentPath, ROUTES } from "./lib/routes";

const TITLES: Record<string, string> = {
  [ROUTES.home]:
    "initagent - control every machine you own from one browser tab",
  [ROUTES.plans]: "Plans - initagent",
  [ROUTES.developers]: "For Developers - initagent",
  [ROUTES.hardware]: "Hardware - initagent",
};

function pageFor(path: string): ReactNode {
  switch (path) {
    case ROUTES.home:
      return <Home />;
    case ROUTES.plans:
      return <Plans />;
    case ROUTES.developers:
      return <Developers />;
    case ROUTES.hardware:
      return <Hardware />;
    default:
      return <NotFound />;
  }
}

export default function App() {
  const path = currentPath();

  useEffect(() => {
    function redirectLegacyHash() {
      if (currentPath() !== ROUTES.home) return;
      if (location.hash === "#plans") {
        location.replace(ROUTES.plans);
        return;
      }
      if (location.hash === "#install") {
        location.replace(ROUTES.developers);
      }
    }
    redirectLegacyHash();
    window.addEventListener("hashchange", redirectLegacyHash);
    return () => window.removeEventListener("hashchange", redirectLegacyHash);
  }, []);

  useEffect(() => {
    document.title = TITLES[path] ?? "initagent";
  }, [path]);

  return (
    <>
      <Nav path={path} />
      {pageFor(path)}
      <Footer />
    </>
  );
}
