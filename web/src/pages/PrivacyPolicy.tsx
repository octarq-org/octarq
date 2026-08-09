import { BrandMark } from "../shell/BrandMark";

export default function PrivacyPolicy() {
  return (
    <div className="octarq-aurora min-h-screen w-full text-foreground p-8">
      <div className="max-w-3xl mx-auto space-y-8">
        <div className="flex items-center gap-3">
          <BrandMark size="md" />
          <span className="font-medium">octarq</span>
        </div>
        <h1 className="text-3xl font-semibold tracking-tight">Privacy Policy</h1>
      </div>
    </div>
  );
}
