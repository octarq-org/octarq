import React from "react";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Field } from "./field";
import { Textarea } from "./textarea";

// A native <textarea> inside a Field takes no part in Base UI's Field context,
// so it renders visually labelled but unnamed for assistive tech. Textarea has
// to be a Field.Control for the label and hint to reach it.
describe("Textarea", () => {
  it("takes its accessible name from the enclosing Field", () => {
    render(
      <Field label="Reserved mailboxes" hint="One per line">
        <Textarea />
      </Field>,
    );
    const control = screen.getByLabelText("Reserved mailboxes");
    expect(control.tagName).toBe("TEXTAREA");
    expect(control.getAttribute("aria-describedby")).toBeTruthy();
  });

  it("keeps its own props on the textarea", () => {
    render(
      <Field label="Note">
        <Textarea rows={5} placeholder="Add a note" defaultValue="hello" />
      </Field>,
    );
    const control = screen.getByLabelText("Note") as HTMLTextAreaElement;
    expect(control.rows).toBe(5);
    expect(control.placeholder).toBe("Add a note");
    expect(control.value).toBe("hello");
  });

  it("renders outside a Field", () => {
    render(<Textarea aria-label="Standalone" />);
    expect(screen.getByLabelText("Standalone").tagName).toBe("TEXTAREA");
  });
});
