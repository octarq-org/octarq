import type { TFunc } from "../i18n";
import type { ChatFailure } from "./command/streamChat";

export function agentFailureText(t: TFunc, failure: ChatFailure | undefined, fallback: string): string {
  switch (failure?.code) {
    case "spend_account_paused":
      return t("copilot.agentPaused", "This account is paused. Resume it in Spend.");
    case "spend_budget_exceeded":
      return t(
        "copilot.agentBudgetExceeded",
        "This account reached its monthly budget limit. Raise the limit in Spend.",
      );
    case "spend_model_price_missing":
      return t("copilot.agentPriceMissing", "The pricing dataset has no price for this model. Choose another model.");
    case "spend_not_recorded":
      return t("copilot.agentSpendNotRecorded", "The turn finished. Its spend was not saved. Check the billing setup.");
    case "spend_account_not_found":
      return t("copilot.agentNotFound", "This account no longer exists. Select another account.");
  }
  if (failure?.status === 403 && failure.code === "forbidden") {
    return t("copilot.agentForbidden", "Only an admin can bill a turn to an account.");
  }
  return fallback;
}
