import { useEffect, useState } from "react";
import { api } from "./client";
import type { Overview } from "./types";

let overviewInflight: { promise: Promise<Overview>; time: number } | null = null;

export function fetchOverview(): Promise<Overview> {
  const now = Date.now();
  if (overviewInflight && now - overviewInflight.time < 2000) {
    return overviewInflight.promise;
  }
  const promise = api.overview().catch((err) => {
    overviewInflight = null;
    throw err;
  });
  overviewInflight = { promise, time: now };
  return promise;
}

export function useOverviewData(): Overview | null {
  const [data, setData] = useState<Overview | null>(null);
  useEffect(() => {
    let active = true;
    fetchOverview()
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
  }, []);
  return data;
}
