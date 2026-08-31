import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { accountMe, replace } = vi.hoisted(() => ({
  accountMe: vi.fn().mockResolvedValue({ authenticated: true }),
  replace: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: { auth: { accountMe } },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
  usePathname: () => "/dashboard",
}));

import { useLoginSessionGate } from "@/lib/use-login-session-gate";

describe("useLoginSessionGate", () => {
  beforeEach(() => {
    accountMe.mockClear();
    replace.mockClear();
    accountMe.mockResolvedValue({ authenticated: true });
  });

  it("probes LoginSession once on mount, not on every path change", async () => {
    const { rerender, result } = renderHook(
      ({ path }: { path: string }) => {
        void path;
        return useLoginSessionGate();
      },
      { initialProps: { path: "/dashboard" } },
    );

    await waitFor(() => expect(result.current.state).toBe("authed"));
    expect(result.current.surface).toBe("account");
    expect(accountMe).toHaveBeenCalledTimes(1);

    rerender({ path: "/models" });
    rerender({ path: "/settings" });
    await waitFor(() => expect(accountMe).toHaveBeenCalledTimes(1));
  });

  it("treats an account cookie as authed", async () => {
    accountMe.mockResolvedValue({
      authenticated: true,
      account_id: "acct_test",
    });

    const { result } = renderHook(() => useLoginSessionGate());

    await waitFor(() => expect(result.current.state).toBe("authed"));
    expect(result.current.surface).toBe("account");
    expect(accountMe).toHaveBeenCalledTimes(1);
    expect(replace).not.toHaveBeenCalled();
  });

  it("bounces to login when accountMe says anonymous", async () => {
    accountMe.mockResolvedValue({ authenticated: false });

    const { result } = renderHook(() => useLoginSessionGate());

    await waitFor(() => expect(result.current.state).toBe("anonymous"));
    expect(result.current.surface).toBe("account");
    expect(replace).toHaveBeenCalledWith("/login?next=%2Fdashboard");
  });

  it("bounces to login when the probe fails", async () => {
    accountMe.mockRejectedValue(new Error("401"));

    const { result } = renderHook(() => useLoginSessionGate());

    await waitFor(() => expect(result.current.state).toBe("anonymous"));
    expect(replace).toHaveBeenCalledWith("/login?next=%2Fdashboard");
  });
});
