import { Switch } from "../base/switch";

// Toggle keeps its `{ on, onChange }` API but is the accessible Base UI Switch
// (role="switch", keyboard-operable, focus-visible ring) instead of a bare
// <button>.
export function Toggle({ on, onChange, disabled }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return <Switch checked={on} onCheckedChange={onChange} />;
}
