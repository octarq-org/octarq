import React from "react";
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Field } from "./field";
import { Textarea } from "./textarea";

// This package's vitest setup has no globals, so testing-library does not
// auto-clean between tests: without this, every rendered textarea stays in the
// document and the label queries below match more than one node.
afterEach(cleanup);

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

  it("stays controlled and fires onChange exactly once", () => {
    // Read the value inside the handler: a controlled textarea snaps back to its
    // prop, so the event object shows the old value once the call returns.
    let seen = "";
    const onChange = vi.fn((e: React.ChangeEvent<HTMLTextAreaElement>) => {
      seen = e.target.value;
    });
    render(
      <Field label="Note">
        <Textarea value="start" onChange={onChange} />
      </Field>,
    );
    const control = screen.getByLabelText("Note") as HTMLTextAreaElement;
    expect(control.value).toBe("start");
    fireEvent.change(control, { target: { value: "next" } });
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(seen).toBe("next");
  });

  it("forwards a ref to the textarea element", () => {
    const ref = React.createRef<HTMLTextAreaElement>();
    render(<Textarea ref={ref} aria-label="Standalone" />);
    expect(ref.current).toBeInstanceOf(HTMLTextAreaElement);
  });

  it("renders outside a Field", () => {
    render(<Textarea aria-label="Standalone" />);
    expect(screen.getByLabelText("Standalone").tagName).toBe("TEXTAREA");
  });
});
