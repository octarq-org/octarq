// Overview fetcher and cached react hook.
import { useEffect, useState } from "react";
import { api } from "./client";
import type { Overview } from "./types";

const overviewInflight = new Map<boolean, { promise: Promise<Overview>; time: number }>();

export function fetchOverview(includeBot = false): Promise<Overview> {
  const key = !!includeBot;
  const now = Date.now();
  const cached = overviewInflight.get(key);
  if (cached && now - cached.time < 2000) {
    return cached.promise;
  }
  const promise = api.overview(key).catch((err) => {
    overviewInflight.delete(key);
    throw err;
  });
  overviewInflight.set(key, { promise, time: now });
  return promise;
}

export function useOverviewData(includeBot = false): Overview | null {
  const [data, setData] = useState<Overview | null>(null);
  useEffect(() => {
    let active = true;
    fetchOverview(includeBot)
      .then((res) => {
        if (active) setData(res);
      })
      .catch((err) => {
        if (import.meta.env.DEV) {
          console.warn("[overview] Failed to fetch overview data:", err);
        }
      });
    return () => {
      active = false;
    };
  }, [includeBot]);
  return data;
}
