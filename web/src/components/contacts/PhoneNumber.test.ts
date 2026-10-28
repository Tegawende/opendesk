import { describe, expect, it } from "vitest";
import { telHref } from "./PhoneNumber";

describe("telHref", () => {
  it("dials numbers as people write them", () => {
    expect(telHref("+32 470 12 34 56")).toBe("tel:+32470123456");
    expect(telHref("0032 (0)470/12.34.56")).toBe("tel:+32470123456");
    expect(telHref("0470 12 34 56")).toBe("tel:0470123456");
  });

  it("refuses what is not a number", () => {
    expect(telHref("")).toBeNull();
    expect(telHref("call me")).toBeNull();
    expect(telHref("12")).toBeNull();
    expect(telHref("47+0")).toBeNull();
    expect(telHref("javascript:alert(1)")).toBeNull();
  });
});
