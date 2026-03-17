import { describe, it, expect } from "vitest";
import { cn, formatBRL } from "./utils";

describe("formatBRL", () => {
  it("formats positive values", () => {
    expect(formatBRL(1234.56)).toBe("R$\u00a01.234,56");
  });

  it("formats negative values", () => {
    expect(formatBRL(-500)).toContain("500");
  });

  it("formats zero", () => {
    expect(formatBRL(0)).toBe("R$\u00a00,00");
  });

  it("returns empty string for null", () => {
    expect(formatBRL(null)).toBe("");
  });

  it("returns empty string for undefined", () => {
    expect(formatBRL(undefined)).toBe("");
  });
});

describe("cn", () => {
  it("merges class names", () => {
    expect(cn("px-2", "py-1")).toBe("px-2 py-1");
  });

  it("handles conditional classes", () => {
    expect(cn("base", false && "hidden", "active")).toBe("base active");
  });

  it("resolves tailwind conflicts", () => {
    expect(cn("px-2", "px-4")).toBe("px-4");
  });
});
