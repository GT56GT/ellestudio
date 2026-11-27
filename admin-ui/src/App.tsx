import { useCallback, useEffect, useState } from "react";
import { ApiError, get, signInWithTelegramApp, signOut, whenSignedOut } from "./api";
import type { Me } from "./types";
import { go, useRoute } from "./router";
import { Tally } from "./components/Tally";
import { IconAudit, IconGroups, IconOverview, IconSessions, IconUsers, Logo } from "./components/Icons";
import { Failed, Loading } from "./components/States";
import { Overview } from "./pages/Overview";
import { Users } from "./pages/Users";
import { UserDetail } from "./pages/UserDetail";
import { Groups } from "./pages/Groups";
import { Audit } from "./pages/Audit";
import { Sessions } from "./pages/Sessions";
import { SignIn } from "./pages/SignIn";

type Auth = { state: "checking" } | { state: "out"; error: string | null } | { state: "in"; me: Me } | { state: "broken"; message: string };

const pages = [
  { path: "/", label: "Overview", title: "Overview", lede: "How Ellie is doing right now and over the last month.", icon: IconOverview },
  { path: "/users", label: "Users", title: "Users", lede: "Everyone who has used Ellie, newest activity first.", icon: IconUsers },
  { path: "/groups", label: "Groups", title: "Groups", lede: "Chats Ellie reads in, busiest today first.", icon: IconGroups },
  { path: "/audit", label: "Audit log", title: "Audit log", lede: "Every sign-in and admin action, newest first.", icon: IconAudit },
  { path: "/sessions", label: "Sessions", title: "Sessions", lede: "Where you're signed in to this panel.", icon: IconSessions },
];

// telegramLaunch returns the Mini App's signed launch data when the panel
// was opened from Ellie's /admin button (Telegram puts it in the URL hash).
function telegramLaunch(): string | null {
  const hash = window.location.hash.slice(1);
  if (!hash.includes("tgWebAppData=")) return null;
  return new URLSearchParams(hash).get("tgWebAppData");
}

export function App() {
  const route = useRoute();
  const [auth, setAuth] = useState<Auth>({ state: "checking" });
  const [botUsername, setBotUsername] = useState("");

  const signedOut = useCallback((error: string | null = null) => setAuth({ state: "out", error }), []);
  useEffect(() => whenSignedOut(() => signedOut()), [signedOut]);

  useEffect(() => {
    let live = true;
    (async () => {
      try {
        const launch = telegramLaunch();
        if (launch) {
          window.history.replaceState(null, "", `${window.location.pathname}#/`);
          await signInWithTelegramApp(launch);
        }
        const me = await get<Me>("/api/me");
        if (live) setAuth({ state: "in", me });
      } catch (e) {
        if (!live) return;
        if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
          signedOut(e.status === 403 ? "not-admin" : null);
        } else {
          setAuth({ state: "broken", message: e instanceof Error ? e.message : "The panel couldn't start." });
        }
      }
    })();
    get<{ bot_username: string }>("/api/auth/config")
      .then((c) => live && setBotUsername(c.bot_username))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [signedOut]);

  if (auth.state === "checking") return <div className="signin"><Loading /></div>;
  if (auth.state === "broken") return <div className="signin"><Failed message={auth.message} retry={() => window.location.reload()} /></div>;
  if (auth.state === "out") return <SignIn botUsername={botUsername} error={route.params.get("error") ?? auth.error} />;

  const userMatch = route.path.match(/^\/users\/(-?\d+)$/);
  const section = userMatch ? "/users" : route.path;
  const page = pages.find((p) => p.path === section) ?? pages[0];

  const doSignOut = async () => {
    await signOut().catch(() => {});
    signedOut();
  };

  return (
    <div className="shell">
      <aside className="rail">
        <div className="brand">
          <Logo />
          <div>
            Ellie Studio
            <small>Admin</small>
          </div>
        </div>
        <nav className="nav" aria-label="Sections">
          {pages.map((p) => (
            <a key={p.path} href={`#${p.path}`} aria-current={p.path === section ? "page" : undefined}>
              <p.icon />
              <span>{p.label}</span>
            </a>
          ))}
        </nav>
        <div className="rail-foot">
          <span>{auth.me.admin.name || `Admin ${auth.me.admin.id}`}</span>
          <ThemeSwitch />
          <button className="link-btn" style={{ textAlign: "left" }} onClick={doSignOut}>
            Sign out
          </button>
        </div>
      </aside>
      <main className="main">
        {!userMatch && (
          <header className="topbar">
            <div>
              <h1 className="page-title">{page.title}</h1>
              <p className="page-lede">{page.lede}</p>
            </div>
            <Tally />
          </header>
        )}
        {userMatch ? (
          <UserDetail id={userMatch[1]} />
        ) : section === "/users" ? (
          <Users route={route} />
        ) : section === "/groups" ? (
          <Groups route={route} />
        ) : section === "/audit" ? (
          <Audit />
        ) : section === "/sessions" ? (
          <Sessions onSignedOut={() => (signedOut(), go("/"))} />
        ) : (
          <Overview />
        )}
      </main>
    </div>
  );
}

// ThemeSwitch cycles the colour scheme: the system's, light, dark. The choice
// is remembered in this browser only.
function ThemeSwitch() {
  const read = () => {
    try {
      return localStorage.getItem("ellie-theme") ?? "system";
    } catch {
      return "system";
    }
  };
  const [theme, setTheme] = useState(read);
  useEffect(() => {
    const root = document.documentElement;
    if (theme === "system") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", theme);
    try {
      localStorage.setItem("ellie-theme", theme);
    } catch {
      /* private mode: the choice lasts for this visit */
    }
  }, [theme]);
  const next = theme === "system" ? "light" : theme === "light" ? "dark" : "system";
  const label = { system: "Theme: match system", light: "Theme: light", dark: "Theme: dark" }[theme] ?? "Theme";
  return (
    <button className="link-btn" style={{ textAlign: "left" }} onClick={() => setTheme(next)}>
      {label}
    </button>
  );
}
