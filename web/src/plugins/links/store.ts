import { useEffect, useState } from "react";

let globalIncludeBot = false;
const listeners = new Set<(val: boolean) => void>();

export function setIncludeBot(val: boolean) {
  globalIncludeBot = val;
  listeners.forEach((l) => l(val));
}

export function useIncludeBot(): [boolean, (val: boolean) => void] {
  const [includeBot, setLocal] = useState(globalIncludeBot);
  useEffect(() => {
    const listener = (val: boolean) => setLocal(val);
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }, []);
  return [includeBot, setIncludeBot];
}
