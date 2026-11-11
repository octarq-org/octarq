import { useEffect, useState } from "react";

declare const process: { env?: Record<string, string | undefined> } | undefined;

export interface StatKV {
  key: string;
  count: number;
}

export interface OverviewData extends Record<string, unknown> {
  tokens?: number;
  includeBot?: boolean;
  links?: number;
  activeLinks?: number;
  domains?: number;
  linkDomains?: number;
  mailDomains?: number;
  mailboxes?: number;
  emails?: number;
  unread?: number;
  totalClicks?: number;
  clicks7d?: number;
  clicks30d?: number;
  botClicks7d?: number;
  botClicks30d?: number;
  series?: StatKV[] | null;
  topLinks?: { id: number; slug: string; host: string; title: string; tags: string; clicks: number }[] | null;
  devices?: StatKV[] | null;
  countries?: StatKV[] | null;
  cities?: StatKV[] | null;
  recentEmails?: { id: number; from: string; subject: string; read: boolean; receivedAt: string }[] | null;
}

const overviewInflight = new Map<boolean, { promise: Promise<OverviewData>; time: number }>();

export function fetchOverview(includeBot = false): Promise<OverviewData> {
  const key = !!includeBot;
  const now = Date.now();
  const cached = overviewInflight.get(key);
  if (cached && now - cached.time < 2000) {
    return cached.promise;
  }
  const promise = fetch(`/api/overview${includeBot ? "?includeBot=true" : ""}`, {
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
      overviewInflight.delete(key);
      throw err;
    });
  overviewInflight.set(key, { promise, time: now });
  return promise;
}

export function useOverviewData(includeBot = false): OverviewData | null {
  const [data, setData] = useState<OverviewData | null>(null);
  useEffect(() => {
    let active = true;
    fetchOverview(includeBot)
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
  }, [includeBot]);
  return data;
}
