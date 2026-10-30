import type { TFunc } from "../i18n";
import type { ChatFailure } from "./command/streamChat";

// agentFailureText turns a billing refusal into the next action an operator
// can take. The server message is the last resort, not the default.
export function agentFailureText(t: TFunc, failure: ChatFailure | undefined, fallback: string): string {
  switch (failure?.code) {
    case "workforce_agent_paused":
      return t("copilot.agentPaused", "This agent is paused. Resume it in AI Workforce.");
    case "workforce_budget_exceeded":
      return t(
        "copilot.agentBudgetExceeded",
        "This agent reached its monthly budget limit. Raise the limit in AI Workforce.",
      );
    case "workforce_model_price_missing":
      return t("copilot.agentPriceMissing", "No price is set for this model. Add the price in AI Workforce.");
    case "workforce_spend_not_recorded":
      return t("copilot.agentSpendNotRecorded", "The turn finished. Its spend was not saved. Check the billing setup.");
    case "workforce_agent_not_found":
      return t("copilot.agentNotFound", "This agent no longer exists. Select another agent.");
  }
  if (failure?.status === 403 && failure.code === "forbidden") {
    return t("copilot.agentForbidden", "Only an admin can bill a turn to an agent.");
  }
  return fallback;
}
