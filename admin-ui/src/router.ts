import { useEffect, useState } from "react";

// Hash routes keep deep links working without server rewrites, and leave the
// query string to Telegram.
export interface Route {
  path: string;
  params: URLSearchParams;
}

function read(): Route {
  const raw = window.location.hash.replace(/^#/, "") || "/";
  const [path, query = ""] = raw.split("?");
  return { path: path || "/", params: new URLSearchParams(query) };
}

export function useRoute() {
  const [route, setRoute] = useState(read);
  useEffect(() => {
    const on = () => setRoute(read());
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);
  return route;
}

export function go(path: string) {
  window.location.hash = path;
}
