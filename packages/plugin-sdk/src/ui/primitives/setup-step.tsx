import { createContext, useContext, useEffect, useId, useState, useCallback, useMemo, ReactNode } from "react";

export interface SetupStepProps {
  title: string;
  description: string;
  completed: boolean;
  onClick: () => void;
}

export interface SetupChecklistRegisterContextType {
  register: (id: string, completed: boolean) => void;
  unregister: (id: string) => void;
}

export interface SetupChecklistStateContextType {
  totalCount: number;
  completedCount: number;
  progressPercent: number;
  allCompleted: boolean;
}

export const SetupChecklistRegisterContext = createContext<SetupChecklistRegisterContextType | null>(null);
export const SetupChecklistStateContext = createContext<SetupChecklistStateContextType | null>(null);

export interface SetupChecklistProviderProps {
  children: ReactNode;
}

export function SetupChecklistProvider({ children }: SetupChecklistProviderProps) {
  const [items, setItems] = useState<Record<string, boolean>>({});

  const register = useCallback((id: string, completed: boolean) => {
    setItems((prev) => {
      if (prev[id] === completed) return prev;
      return { ...prev, [id]: completed };
    });
  }, []);

  const unregister = useCallback((id: string) => {
    setItems((prev) => {
      if (!(id in prev)) return prev;
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }, []);

  const totalCount = Object.keys(items).length;
  const completedCount = Object.values(items).filter(Boolean).length;
  const progressPercent = totalCount > 0 ? Math.round((completedCount / totalCount) * 100) : 0;
  const allCompleted = totalCount > 0 && completedCount === totalCount;

  const registerValue = useMemo(
    () => ({ register, unregister }),
    [register, unregister]
  );

  const stateValue = useMemo(
    () => ({ totalCount, completedCount, progressPercent, allCompleted }),
    [totalCount, completedCount, progressPercent, allCompleted]
  );

  return (
    <SetupChecklistRegisterContext.Provider value={registerValue}>
      <SetupChecklistStateContext.Provider value={stateValue}>
        {children}
      </SetupChecklistStateContext.Provider>
    </SetupChecklistRegisterContext.Provider>
  );
}

export function useSetupChecklist(): SetupChecklistStateContextType {
  const ctx = useContext(SetupChecklistStateContext);
  if (!ctx) {
    return {
      totalCount: 0,
      completedCount: 0,
      progressPercent: 0,
      allCompleted: false,
    };
  }
  return ctx;
}

export function SetupStep({ title, description, completed, onClick }: SetupStepProps) {
  const id = useId();
  const registerCtx = useContext(SetupChecklistRegisterContext);

  useEffect(() => {
    if (registerCtx) {
      registerCtx.register(id, completed);
      return () => {
        registerCtx.unregister(id);
      };
    }
  }, [id, completed, registerCtx]);

  return (
    <button
      onClick={onClick}
      className={`group flex flex-col text-left p-4 rounded-xl border transition-all duration-200 ${
        completed
          ? "bg-well border-success-border/50 hover:border-success-border"
          : "bg-well border-border hover:border-accent-border hover:bg-surface-hover"
      }`}
    >
      <div className="flex items-center justify-between w-full">
        <div className={`p-1.5 rounded-lg ${completed ? "text-success-fg bg-success-bg" : "text-accent-fg bg-accent-soft"}`}>
          {completed ? (
            <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <path d="m9 12 2 2 4-4" />
            </svg>
          ) : (
            <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
            </svg>
          )}
        </div>
        {!completed && (
          <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="text-foreground/0 group-hover:text-accent-fg translate-x-[-4px] group-hover:translate-x-0 transition-all duration-200">
            <path d="M5 12h14" />
            <path d="m12 5 7 7-7 7" />
          </svg>
        )}
      </div>
      <h3 className={`font-semibold text-sm mt-3 ${completed ? "text-muted-foreground line-through" : "text-foreground"}`}>
        {title}
      </h3>
      <p className="text-[11px] text-muted-foreground mt-1 leading-normal flex-1">
        {description}
      </p>
    </button>
  );
}
