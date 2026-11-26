import { describe, it, expect } from "vitest";
import type { UIArea } from "@octarq/plugin-sdk";
import { areaForCategory, FOOTER_PLACEMENT } from "./areas";

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

  it("routes an unclaimed category to operations", () => {
    expect(areaForCategory("Storage & Databases", [COMMERCE])).toBe("operations");
    expect(areaForCategory("Messaging", [COMMERCE])).toBe("operations");
    expect(areaForCategory(undefined, [COMMERCE])).toBe("operations");
  });

  it("routes a category a plugin area declares", () => {
    const assets: UIArea = { id: "assets", title: "Infrastructure", groups: ["Storage & Databases"] };
    expect(areaForCategory("Storage & Databases", [COMMERCE, assets])).toBe("assets");
  });

  it("uses deterministic group matching and eliminates fuzzy substring heuristics", () => {
    expect(areaForCategory("asset-tracker", [COMMERCE])).toBe("operations");
    expect(areaForCategory("cloud-compute-nodes", [COMMERCE])).toBe("operations");
    expect(areaForCategory("security-audit-center", [COMMERCE])).toBe("operations");
  });
});
