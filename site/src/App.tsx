import { useEffect, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Nav } from "./components/Nav";
import { Footer } from "./components/Footer";
import { Home } from "./components/Home";
import { Plans } from "./components/Plans";
import { Developers } from "./components/Developers";
import { Hardware } from "./components/Hardware";
import { NotFound } from "./components/NotFound";
import { currentPath, ROUTES } from "./lib/routes";

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
  const { t } = useTranslation();

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
    const titles: Record<string, string> = {
      [ROUTES.home]: t("titles.home"),
      [ROUTES.plans]: t("titles.plans"),
      [ROUTES.developers]: t("titles.developers"),
      [ROUTES.hardware]: t("titles.hardware"),
    };
    document.title = titles[path] ?? "initagent";
  }, [path, t]);

  return (
    <>
      <Nav path={path} />
      {pageFor(path)}
      <Footer />
    </>
  );
}
