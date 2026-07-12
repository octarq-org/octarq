// Placeholder pages for infrastructure features on the roadmap (surfaced by the
// core "assets" plugin — see plugins/core/assets.ts). Each export is a
// zero-props page so it fits the UIPlugin LazyPage contract.
import { Boxes } from "lucide-react";
import { GlassCard, PageHeader, ScreenWrap } from "../ui";
import { useTranslation } from "../i18n";

function ComingSoonPage({ title, description }: { title: string; description: string }) {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <PageHeader title={title} description={description} />
      <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center">
        <div className="h-16 w-16 rounded-2xl bg-indigo-500/10 flex items-center justify-center text-indigo-400 mb-4 animate-pulse">
          <Boxes className="h-8 w-8" />
        </div>
        <h3 className="text-lg font-bold text-white mb-2">{t("app.comingSoonTitle")}</h3>
        <p className="text-sm text-white/50 max-w-sm leading-relaxed">
          {t("app.comingSoonBody")}
        </p>
      </GlassCard>
    </ScreenWrap>
  );
}

export function CertificatesComingSoon() {
  const { t } = useTranslation();
  return <ComingSoonPage title={t("app.certsTitle")} description={t("app.certsDesc")} />;
}

export function DatabasesComingSoon() {
  const { t } = useTranslation();
  return <ComingSoonPage title={t("app.databasesTitle")} description={t("app.databasesDesc")} />;
}

export function StorageComingSoon() {
  const { t } = useTranslation();
  return <ComingSoonPage title={t("app.storageTitle")} description={t("app.storageDesc")} />;
}
