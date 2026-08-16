// @vitest-environment happy-dom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { FormError, formErrorMessage, formErrorStatusKeys } from "./FormError";

// The t() shim mirrors the SDK TFunc shape without touching real dictionaries.
const t = (key: string) => `t:${key}`;

describe("formErrorMessage", () => {
  it("maps known failure statuses to their localized copy", () => {
    expect(formErrorMessage({ message: "backend raw", status: 403 }, t)).toBe("t:uiCommon.errStatus403");
    expect(formErrorMessage({ message: "backend raw", status: 429 }, t)).toBe("t:uiCommon.errStatus429");
    expect(formErrorMessage({ message: "backend raw", status: 500 }, t)).toBe("t:uiCommon.errStatus500");
  });

  it("falls back to the backend's original message for unknown statuses (never swallowed)", () => {
    const raw = "the workspace address could not be claimed — please try again";
    expect(formErrorMessage({ message: raw, status: 409 }, t)).toBe(raw);
    expect(formErrorMessage({ message: raw, status: 418 }, t)).toBe(raw);
  });

  it("passes string errors straight through", () => {
    expect(formErrorMessage("plain string error", t)).toBe("plain string error");
  });

  it("exposes no mapping for unknown statuses", () => {
    expect(formErrorStatusKeys[409]).toBeUndefined();
  });
});

describe("FormError", () => {
  it("renders the localized copy for a mapped status", () => {
    render(
      <I18nProvider>
        <FormError err={{ message: "backend raw", status: 403, requestId: "req-1" }} />
      </I18nProvider>
    );
    expect(screen.getByText("You don't have permission to do that.")).toBeTruthy();
    expect(screen.getByText(/HTTP 403/)).toBeTruthy();
  });

  it("renders the backend original for an unmapped status", () => {
    const raw = "the workspace address could not be claimed — please try again";
    render(
      <I18nProvider>
        <FormError err={{ message: raw, status: 409, requestId: "req-2" }} />
      </I18nProvider>
    );
    expect(screen.getByText(raw)).toBeTruthy();
  });
});
