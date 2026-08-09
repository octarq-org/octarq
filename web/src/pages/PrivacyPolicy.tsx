import { BrandMark } from "../shell/BrandMark";

export default function PrivacyPolicy() {
  return (
    <div className="octarq-aurora min-h-screen w-full text-foreground p-8">
      <div className="max-w-3xl mx-auto space-y-8">
        <div className="flex items-center gap-3">
          <BrandMark size="md" />
          <span className="font-medium">octarq</span>
        </div>
        
        <div className="space-y-4">
          <h1 className="text-3xl font-semibold tracking-tight">Privacy Policy</h1>
          <p className="text-muted-foreground">Last updated: {new Date().toLocaleDateString()}</p>
          
          <div className="prose prose-sm dark:prose-invert max-w-none">
            <p>
              This is the placeholder Privacy Policy for octarq.
            </p>
            <h2>1. Information Collection</h2>
            <p>We collect information to provide better services to all our users.</p>
            <h2>2. Use of Information</h2>
            <p>We use the information we collect from all our services to provide, maintain, protect and improve them.</p>
          </div>
        </div>
      </div>
    </div>
  );
}
