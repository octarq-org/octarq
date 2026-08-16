import { describe, it, expect } from "vitest";
import type { UIArea } from "@octarq/plugin-sdk";
import { areaForCategory, FOOTER_PLACEMENT } from "./areas";

// A plugin places a menu by setting Category in its Go half. areaForCategory
// resolves that string to an area id. What it may resolve *against* is the
// contract in website/src/content/docs/writing-a-plugin.md: the area's id, or
// one of its declared group labels.
//
// Title used to work too, which made a display string load-bearing: `title` is
// what the sidebar renders and what translateAreaTitle localizes, so renaming
// an area — the exact thing #134 asks about — would silently relocate every
// menu that named it, with nothing failing to say so.
const COMMERCE: UIArea = {
  id: "commerce",
  title: "Commerce",
  subtitle: "Revenue, store & cost analysis",
  icon: "wallet",
  groups: ["Sales", "Finance"],
};

describe("areaForCategory", () => {
  it("places a menu by the area id", () => {
    expect(areaForCategory("commerce", [COMMERCE])).toBe("commerce");
    expect(areaForCategory("COMMERCE", [COMMERCE])).toBe("commerce");
  });

  it("places a menu by a declared group label", () => {
    expect(areaForCategory("Sales", [COMMERCE])).toBe("commerce");
    expect(areaForCategory("finance", [COMMERCE])).toBe("commerce");
  });

  // The guard. Commerce's title happens to equal its id here, as it does in the
  // real Pro declaration, so a title match is indistinguishable from an id
  // match — give the area a title that resolves nowhere else.
  it("does NOT place a menu by the area title", () => {
    const renamed: UIArea = { ...COMMERCE, title: "Storefront" };
    expect(areaForCategory("Storefront", [renamed])).not.toBe("commerce");
  });

  it("keeps renaming an area title from moving its menus", () => {
    const before = areaForCategory("Sales", [COMMERCE]);
    const after = areaForCategory("Sales", [{ ...COMMERCE, title: "Money" }]);
    expect(after).toBe(before);
  });

  it("still routes footer and settings categories ahead of any area", () => {
    expect(areaForCategory("footer", [COMMERCE])).toBe(FOOTER_PLACEMENT);
    expect(areaForCategory("resources", [COMMERCE])).toBe(FOOTER_PLACEMENT);
    expect(areaForCategory("Instance", [COMMERCE])).toBe("settings");
    expect(areaForCategory("Account", [COMMERCE])).toBe("settings");
  });

  it("falls back to keyword routing for an unclaimed category", () => {
    expect(areaForCategory("Storage & Databases", [COMMERCE])).toBe("assets");
    expect(areaForCategory("Messaging", [COMMERCE])).toBe("operations");
    expect(areaForCategory(undefined, [COMMERCE])).toBe("operations");
  });
});
