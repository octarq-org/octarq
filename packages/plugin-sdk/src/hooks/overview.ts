import { useEffect, useState } from "react";

declare const process: { env?: Record<string, string | undefined> } | undefined;

export interface OverviewData extends Record<string, unknown> {
  tokens?: number;
}

let overviewInflight: { promise: Promise<OverviewData>; time: number } | null = null;

export function fetchOverview(): Promise<OverviewData> {
  const now = Date.now();
  if (overviewInflight && now - overviewInflight.time < 2000) {
    return overviewInflight.promise;
  }
  const promise = fetch("/api/overview", {
    credentials: "same-origin",
    headers: {
      "Accept": "application/json",
    },
  })
    .then(async (res) => {
      if (!res.ok) {
        throw new Error(`Failed to fetch overview: ${res.status}`);
      }
      return res.json() as Promise<OverviewData>;
    })
    .catch((err) => {
      overviewInflight = null;
      throw err;
    });
  overviewInflight = { promise, time: now };
  return promise;
}

export function useOverviewData(): OverviewData | null {
  const [data, setData] = useState<OverviewData | null>(null);
  useEffect(() => {
    let active = true;
    fetchOverview()
      .then((res) => {
        if (active) setData(res);
      })
      .catch((err) => {
        if (typeof process !== "undefined" && process.env?.NODE_ENV !== "production") {
          console.warn("[overview] Failed to fetch overview data:", err);
        }
      });
    return () => {
      active = false;
    };
  }, []);
  return data;
}
