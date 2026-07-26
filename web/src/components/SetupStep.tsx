import { CheckCircle2, Circle, ArrowRight } from "lucide-react";

export interface SetupStepProps {
  title: string;
  description: string;
  completed: boolean;
  onClick: () => void;
}

export function SetupStep({ title, description, completed, onClick }: SetupStepProps) {
  return (
    <button
      onClick={onClick}
      className={`group flex flex-col text-left p-4 rounded-xl border transition-all duration-200 ${
        completed 
          ? "bg-foreground/[0.02] border-success-border/50 hover:border-success-border" 
          : "bg-foreground/5 border-foreground/[0.06] hover:border-indigo-500/30 hover:bg-foreground/[0.08]"
      }`}
    >
      <div className="flex items-center justify-between w-full">
        <div className={`p-1.5 rounded-lg ${completed ? "text-success-fg bg-success-bg" : "text-accent-fg bg-indigo-500/10"}`}>
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
