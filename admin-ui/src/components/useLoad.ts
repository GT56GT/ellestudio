import { useCallback, useEffect, useState } from "react";
import { ApiError, get } from "../api";

// useLoad fetches path and refetches when it changes or every refresh ms.
export function useLoad<T>(path: string, refresh = 0) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const reload = useCallback(() => setTick((t) => t + 1), []);

  useEffect(() => {
    let live = true;
    get<T>(path)
      .then((d) => live && (setData(d), setError(null)))
      .catch((e: unknown) => live && setError(e instanceof ApiError ? e.message : "Couldn't load this."));
    return () => {
      live = false;
    };
  }, [path, tick]);

  useEffect(() => {
    if (!refresh) return;
    const id = window.setInterval(() => {
      if (document.visibilityState === "visible") reload();
    }, refresh);
    return () => window.clearInterval(id);
  }, [refresh, reload]);

  return { data, error, reload };
}
