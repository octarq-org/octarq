// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { Login } from "./Login";
import InviteAcceptPage from "../pages/InviteAccept";
import ResetPasswordPage from "../pages/ResetPassword";

function renderPage(ui: React.ReactElement) {
  render(
    <MemoryRouter>
      <I18nProvider>{ui}</I18nProvider>
    </MemoryRouter>,
  );
}

function expectNoCommentTextInPage() {
  const html = document.body.innerHTML;
  expect(html).not.toContain("/*");
  expect(html).not.toContain("*/");
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("auth pages never render suppression markers as text", () => {
  it("Login renders without /* or */ in its output", () => {
    vi.spyOn(api, "authConfig").mockResolvedValue({
      googleEnabled: false,
      githubEnabled: false,
      registrationEnabled: true,
      appName: "Octarq",
      logoUrl: "",
      brandColor: "",
      brandColor2: "",
    });
    renderPage(<Login onLogin={() => {}} />);
    expectNoCommentTextInPage();
  });

  it("InviteAccept renders without /* or */ in its output", () => {
    renderPage(<InviteAcceptPage />);
    expectNoCommentTextInPage();
  });

  it("ResetPassword renders without /* or */ in its output", () => {
    renderPage(<ResetPasswordPage />);
    expectNoCommentTextInPage();
  });
});
