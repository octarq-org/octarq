import { createContext, useContext, useEffect, useId, useState, useCallback, useMemo, ReactNode } from "react";
import { CheckCircle2, Circle, ArrowRight } from "lucide-react";

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
          ? "bg-foreground/[0.02] border-success-border/50 hover:border-success-border" 
          : "bg-foreground/5 border-foreground/[0.06] hover:border-indigo-500/30 hover:bg-foreground/[0.08]" /* ui-color-ok */
      }`}
    >
      <div className="flex items-center justify-between w-full">
        <div className={`p-1.5 rounded-lg ${completed ? "text-success-fg bg-success-bg" : "text-accent-fg bg-indigo-500/10"}`} /* ui-color-ok */>
          {completed ? <CheckCircle2 size={16} /> : <Circle size={16} />}
        </div>
        {!completed && (
          <ArrowRight size={14} className="text-foreground/0 group-hover:text-accent-fg translate-x-[-4px] group-hover:translate-x-0 transition-all duration-200" />
        )}
      </div>
      <h3 className={`font-semibold text-sm mt-3 ${completed ? "text-foreground/60 line-through" : "text-foreground"}`}>
        {title}
      </h3>
      <p className="text-[11px] text-foreground/40 mt-1 leading-normal flex-1">
        {description}
      </p>
    </button>
  );
}
