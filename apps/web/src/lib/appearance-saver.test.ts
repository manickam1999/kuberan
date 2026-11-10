import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AppearanceSaver,
  type ConfirmedAppearance,
  type SaveHandlers,
} from "./appearance-saver";
import type { Appearance } from "@/types/models";

const start: ConfirmedAppearance = {
  appearance: { palette: "nord", mode: "dark" },
  resolved_palette: "nord",
};

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function handlers(send: SaveHandlers["send"]) {
  return {
    send: vi.fn(send),
    onSaved: vi.fn(),
    onFailed: vi.fn(),
  };
}

const echo = async (a: Appearance): Promise<ConfirmedAppearance> => ({
  appearance: a,
  resolved_palette: a.palette,
});

describe("AppearanceSaver", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("merges quick changes into one request carrying every field", async () => {
    const saver = new AppearanceSaver(350);
    const h = handlers(echo);
    saver.schedule({ palette: "gruvbox" }, start, h);
    saver.schedule({ mode: "light" }, start, h);
    saver.schedule({ palette: "cobalt" }, start, h);
    expect(h.send).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(350);
    expect(h.send).toHaveBeenCalledTimes(1);
    expect(h.send).toHaveBeenCalledWith({ palette: "cobalt", mode: "light" });
    expect(h.onSaved).toHaveBeenCalledWith({
      appearance: { palette: "cobalt", mode: "light" },
      resolved_palette: "cobalt",
    });
    expect(saver.getStatus()).toBe("saved");
  });

  it("ignores a late reply for a choice the user has replaced", async () => {
    const saver = new AppearanceSaver(10);
    const first = deferred<ConfirmedAppearance>();
    const h = handlers(echo);
    h.send.mockImplementationOnce(() => first.promise);

    saver.schedule({ palette: "gruvbox" }, start, h);
    await vi.advanceTimersByTimeAsync(10); // request 1 in flight
    saver.schedule({ palette: "cobalt" }, start, h);
    await vi.advanceTimersByTimeAsync(10); // request 2 sent and answered
    expect(h.onSaved).toHaveBeenLastCalledWith({
      appearance: { palette: "cobalt" },
      resolved_palette: "cobalt",
    });

    first.resolve({ appearance: { palette: "gruvbox" }, resolved_palette: "gruvbox" });
    await vi.runAllTimersAsync();
    expect(h.onSaved).toHaveBeenCalledTimes(1); // gruvbox never re-applied
  });

  it("restores the last confirmed state when the newest save fails", async () => {
    const saver = new AppearanceSaver(10);
    const h = handlers(() => Promise.reject(new Error("offline")));
    saver.schedule({ palette: "gruvbox" }, start, h);
    await vi.advanceTimersByTimeAsync(10);
    expect(h.onFailed).toHaveBeenCalledWith(start);
    expect(h.onSaved).not.toHaveBeenCalled();
    expect(saver.getStatus()).toBe("error");
  });

  it("does not revert when an older request fails but a newer one succeeds", async () => {
    const saver = new AppearanceSaver(10);
    const first = deferred<ConfirmedAppearance>();
    const h = handlers(echo);
    h.send.mockImplementationOnce(() => first.promise);

    saver.schedule({ palette: "gruvbox" }, start, h);
    await vi.advanceTimersByTimeAsync(10);
    saver.schedule({ palette: "cobalt" }, start, h);
    await vi.advanceTimersByTimeAsync(10);
    first.reject(new Error("timeout"));
    await vi.runAllTimersAsync();
    expect(h.onFailed).not.toHaveBeenCalled();
    expect(saver.getStatus()).toBe("saved");
  });

  it("rolls back to the newest confirmed state, not the first one", async () => {
    const saver = new AppearanceSaver(10);
    const h = handlers(echo);
    saver.schedule({ palette: "gruvbox" }, start, h);
    await vi.advanceTimersByTimeAsync(10);
    h.send.mockImplementationOnce(() => Promise.reject(new Error("500")));
    saver.schedule({ palette: "cobalt" }, start, h);
    await vi.advanceTimersByTimeAsync(10);
    expect(h.onFailed).toHaveBeenCalledWith({
      appearance: { palette: "gruvbox" },
      resolved_palette: "gruvbox",
    });
  });

  it("notifies subscribers of status changes", async () => {
    const saver = new AppearanceSaver(10);
    const seen: string[] = [];
    saver.subscribe(() => seen.push(saver.getStatus()));
    saver.schedule({ mode: "light" }, start, handlers(echo));
    await vi.advanceTimersByTimeAsync(10);
    expect(seen).toEqual(["saving", "saved"]);
  });
});
