import { useCallback, useEffect, useRef, useState } from "react";
import { api, type WorkforceAgentDTO } from "../api";

// useWorkforceAgents loads the workforce agents an admin can bill a turn to,
// plus the workspace auto-halt switch. The switch decides whether a budget
// limit blocks a turn, so a warning about that limit must read it.
//
// Failures are reported, never thrown: attribution is optional, and the chat
// keeps working unbilled when the list cannot load.
export function useWorkforceAgents(enabled: boolean) {
  const [agents, setAgents] = useState<WorkforceAgentDTO[]>([]);
  const [autoHalt, setAutoHalt] = useState<boolean | null>(null);
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    try {
      // The overview is a separate read: when it fails, the agent list still
      // renders and autoHalt stays null, which suppresses budget warnings the
      // client cannot verify.
      const [list, overview] = await Promise.all([
        api.workforceAgents(),
        Promise.resolve(api.workforceOverview()).catch(() => null),
      ]);
      if (!mounted.current) return;
      setAgents(list);
      setAutoHalt(overview ? overview.autoHalt : null);
      setFailed(false);
    } catch {
      if (!mounted.current) return;
      setFailed(true);
    } finally {
      if (mounted.current) setLoaded(true);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
  }, [enabled, refresh]);

  return { agents, autoHalt, failed, loaded, refresh };
}
