import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api";

// useDefaultAgent loads and saves the agent an unattributed chat turn is
// billed to. The setting lives in the workspace, so every chat surface bills
// the same agent and the answer survives a reload.
//
// A load failure is reported, never thrown: the chat keeps working and the
// turn stays unbilled, which is what the server does when no default is set.
export function useDefaultAgent(enabled: boolean) {
  const [agentId, setAgentId] = useState("");
  // name lets a member see where a turn is billed: the agent list is an admin
  // read, so the setting carries the label with the id.
  const [name, setName] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    try {
      const res = await api.workforceDefaultAgent();
      if (!mounted.current) return;
      setAgentId(res.agentId ?? "");
      setName(res.name ?? "");
      setFailed(false);
    } catch {
      if (mounted.current) setFailed(true);
    } finally {
      if (mounted.current) setLoaded(true);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
  }, [enabled, refresh]);

  // save writes the setting and rolls back on failure, so the picker never
  // shows a choice the server did not keep.
  const save = useCallback(
    async (next: string, nextName: string) => {
      const previous = { agentId, name };
      setAgentId(next);
      setName(nextName);
      try {
        const res = await api.setWorkforceDefaultAgent(next);
        if (mounted.current) {
          setAgentId(res.agentId ?? "");
          setName(res.name ?? "");
        }
        return true;
      } catch {
        if (mounted.current) {
          setAgentId(previous.agentId);
          setName(previous.name);
        }
        return false;
      }
    },
    [agentId, name]
  );

  return { agentId, name, loaded, failed, refresh, save };
}
