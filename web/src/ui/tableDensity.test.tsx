// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { fireEvent, render } from "@testing-library/react";
import { TD, TableDensityProvider, useSetTableDensity } from "@octarq/plugin-sdk";

const TSX_MODULES = import.meta.glob("../**/*.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

describe("table density & primitives guard", () => {
  it("applies density padding correctly to TD and defaults to comfortable without provider", () => {
    const { container: noProvider } = render(<table><tbody><tr><TD>No Provider</TD></tr></tbody></table>);
    const { container: comfortable } = render(
      <TableDensityProvider density="comfortable">
        <table><tbody><tr><TD>Comfortable</TD></tr></tbody></table>
      </TableDensityProvider>
    );
    const { container: compact } = render(
      <TableDensityProvider density="compact">
        <table><tbody><tr><TD>Compact</TD></tr></tbody></table>
      </TableDensityProvider>
    );

    const noProvClass = noProvider.querySelector("td")?.className;
    const comfClass = comfortable.querySelector("td")?.className;
    const compClass = compact.querySelector("td")?.className;

    expect(noProvClass).toEqual(comfClass);
    expect(compClass).not.toEqual(comfClass);
    expect(compClass).toContain("py-1");
    expect(comfClass).toContain("py-2.5");
  });

  it("hands the preferences UI a setter through the same provider", () => {
    // The provider carries the density down and the setter back up, so a
    // preferences control needs no second channel. Outside a provider the
    // setter is an inert no-op rather than a crash.
    function Control() {
      const setDensity = useSetTableDensity();
      return <button onClick={() => setDensity("compact")}>set</button>;
    }
    const seen: string[] = [];
    const { container, getByText } = render(
      <TableDensityProvider density="comfortable" onDensityChange={(d) => seen.push(d)}>
        <Control />
        <table><tbody><tr><TD>Row</TD></tr></tbody></table>
      </TableDensityProvider>,
    );
    expect(container.querySelector("td")?.className).toContain("py-2.5");
    fireEvent.click(getByText("set"));
    expect(seen).toEqual(["compact"]);

    // Scope to this render's own container — both trees share document.body.
    const orphan = render(<Control />);
    const orphanButton = orphan.container.querySelector("button")!;
    expect(() => fireEvent.click(orphanButton)).not.toThrow();
  });

  it("prohibits raw <table>, <thead>, and <tbody> elements in web/src", () => {
    const BARE_TABLE_REGEX = /<(table|thead|tbody)[\s>]/;
    const offenders: string[] = [];

    for (const [file, code] of Object.entries(TSX_MODULES)) {
      if (file.endsWith(".test.tsx") || file.endsWith(".test.ts")) continue;
      // Strip comments so code comments don't trigger match
      const scannable = code.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (c) =>
        c.replace(/[^\n]/g, " "),
      );
      scannable.split("\n").forEach((line, i) => {
        if (BARE_TABLE_REGEX.test(line)) {
          offenders.push(`${file}:${i + 1}  ${line.trim()}`);
        }
      });
    }

    expect(
      offenders,
      `Raw <table>, <thead>, or <tbody> tags are forbidden in web/src. Use Table, THead, TBody primitives from @octarq/plugin-sdk instead.\n${offenders.join("\n")}`,
    ).toEqual([]);
  });
});
