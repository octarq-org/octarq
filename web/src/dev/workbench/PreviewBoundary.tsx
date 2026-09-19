import { Component, type ReactNode } from "react";

interface PreviewBoundaryProps {
  name: string;
  children: ReactNode;
}

interface PreviewBoundaryState {
  error: Error | null;
}

export class PreviewBoundary extends Component<PreviewBoundaryProps, PreviewBoundaryState> {
  state: PreviewBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): PreviewBoundaryState {
    return { error };
  }

  render() {
    const { error } = this.state;
    if (error) {
      return (
        <div className="rounded-md border border-danger-border bg-danger-bg p-3 text-sm text-danger-fg">
          <p className="font-medium">{this.props.name} preview threw</p>
          <pre className="mt-2 overflow-x-auto font-mono text-[11px]">{error.message}</pre>
        </div>
      );
    }
    return this.props.children;
  }
}
