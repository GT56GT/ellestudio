import { useEffect, useRef } from "react";
import { Logo } from "../components/Icons";

const errors: Record<string, string> = {
  "not-admin": "That Telegram account isn't a studio admin. Sign in with an account listed in ADMIN_USER_IDS.",
  failed: "Telegram's sign-in didn't go through. Try again.",
  "slow-down": "Too many sign-in attempts. Wait a few minutes, then try again.",
  unavailable: "The panel can't reach its database right now. Try again in a minute.",
};

export function SignIn({ botUsername, error }: { botUsername: string; error: string | null }) {
  const slot = useRef<HTMLDivElement>(null);

  // Telegram's Login Widget, in redirect mode: after the user confirms in
  // Telegram, the browser comes back to /api/auth/telegram with a signed
  // proof, and the server sets the session cookie.
  useEffect(() => {
    const el = slot.current;
    if (!el || !botUsername) return;
    const s = document.createElement("script");
    s.async = true;
    s.src = "https://telegram.org/js/telegram-widget.js?22";
    s.dataset.telegramLogin = botUsername;
    s.dataset.size = "large";
    s.dataset.radius = "8";
    s.dataset.authUrl = `${window.location.origin}/api/auth/telegram`;
    el.replaceChildren(s);
    return () => el.replaceChildren();
  }, [botUsername]);

  return (
    <main className="signin">
      <div className="panel signin-card">
        <Logo />
        <h1>Ellie Studio admin</h1>
        <p>Sign in with the Telegram account you use to run Ellie.</p>
        {error && (
          <div className="notice" data-tone="hot" role="alert">
            {errors[error] ?? errors.failed}
          </div>
        )}
        <div className="widget-slot" ref={slot} />
        <p className="muted" style={{ marginTop: 20, fontSize: 13 }}>
          On your phone, open Ellie in Telegram, send /admin and tap Open admin panel. You're signed in without this step.
        </p>
      </div>
    </main>
  );
}
