import { describe, it, expect } from "vitest";
import { formatDuration, formatSpeed, formatFileSize, formatCapacity, formatRelativeTime } from "./format";

describe("formatDuration", () => {
  it("defaults to seconds with 2 decimals", () => {
    expect(formatDuration(1500)).toBe("1.50s");
    expect(formatDuration(0)).toBe("0.00s");
    expect(formatDuration(250)).toBe("0.25s");
  });

  it("honors custom precision", () => {
    expect(formatDuration(1500, { precision: 1 })).toBe("1.5s");
  });

  it("renders sub-second durations as ms when subSecondMs is set", () => {
    expect(formatDuration(850, { precision: 1, subSecondMs: true })).toBe("850ms");
    expect(formatDuration(1500, { precision: 1, subSecondMs: true })).toBe("1.5s");
    expect(formatDuration(999, { subSecondMs: true })).toBe("999ms");
  });
});

describe("formatSpeed", () => {
  it("formats tokens per second", () => {
    expect(formatSpeed(42.5)).toBe("42.50 t/s");
    expect(formatSpeed(0)).toBe("0.00 t/s");
  });

  it("reports negative values as unknown", () => {
    expect(formatSpeed(-1)).toBe("unknown");
  });
});

describe("formatFileSize", () => {
  it("formats bytes", () => {
    expect(formatFileSize(512)).toBe("512 B");
  });

  it("formats kilobytes", () => {
    expect(formatFileSize(2048)).toBe("2.0 KB");
  });

  it("formats megabytes", () => {
    expect(formatFileSize(5 * 1024 * 1024)).toBe("5.0 MB");
  });
});

describe("formatCapacity", () => {
  it("formats binary hardware capacities", () => {
    expect(formatCapacity(24 * 1024 ** 3)).toBe("24.0 GiB");
    expect(formatCapacity(1.5 * 1024 ** 4)).toBe("1.50 TiB");
  });

  it("handles unavailable capacities", () => {
    expect(formatCapacity(0)).toBe("Not detected");
  });
});

describe("formatRelativeTime", () => {
  it("always uses an absolute local date and time", () => {
    const recent = new Date(2026, 5, 28, 11, 59, 58);
    const older = new Date(2026, 5, 25, 12, 34, 56);
    expect(formatRelativeTime(recent.toISOString())).toBe("2026-06-28 11:59:58");
    expect(formatRelativeTime(older.toISOString())).toBe("2026-06-25 12:34:56");
    expect(formatRelativeTime("invalid")).toBe("—");
  });
});
