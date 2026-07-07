import { Switch } from "../base/switch";

// Toggle keeps its `{ on, onChange }` API but is the accessible Base UI Switch
// (role="switch", keyboard-operable, focus-visible ring) instead of a bare
// <button>.
export function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return <Switch checked={on} onCheckedChange={onChange} />;
}
