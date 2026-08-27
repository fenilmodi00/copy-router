import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { me, accountMe, replace } = vi.hoisted(() => ({
  me: vi.fn().mockResolvedValue({ authenticated: true }),
  accountMe: vi.fn().mockRejectedValue(new Error("404: not mounted")),
  replace: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: { auth: { me, accountMe } },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
  usePathname: () => "/dashboard",
}));

import { useLoginSessionGate } from "@/lib/use-login-session-gate";

describe("useLoginSessionGate", () => {
  beforeEach(() => {
    me.mockClear();
    accountMe.mockClear();
    replace.mockClear();
    // Default = selfhosted: account surface absent, admin me authed.
    accountMe.mockRejectedValue(new Error("404: not mounted"));
    me.mockResolvedValue({ authenticated: true });
  });

  it("probes LoginSession once on mount, not on every path change", async () => {
    const { rerender, result } = renderHook(
      ({ path }: { path: string }) => {
        void path;
        return useLoginSessionGate();
      },
      { initialProps: { path: "/dashboard" } },
    );

    await waitFor(() => expect(result.current).toBe("authed"));
    expect(accountMe).toHaveBeenCalledTimes(1);
    expect(me).toHaveBeenCalledTimes(1);

    rerender({ path: "/models" });
    rerender({ path: "/settings" });
    await waitFor(() => expect(accountMe).toHaveBeenCalledTimes(1));
    expect(me).toHaveBeenCalledTimes(1);
  });

  it("treats a selfserve account cookie as authed without calling admin me", async () => {
    // Regression: admin /auth/me is 404 in selfserve; probing it alone
    // bounced to /login while login's accountMe bounced back to /dashboard.
    accountMe.mockResolvedValue({
      authenticated: true,
      account_id: "acct_test",
    });

    const { result } = renderHook(() => useLoginSessionGate());

    await waitFor(() => expect(result.current).toBe("authed"));
    expect(accountMe).toHaveBeenCalledTimes(1);
    expect(me).not.toHaveBeenCalled();
    expect(replace).not.toHaveBeenCalled();
  });

  it("bounces to login when selfserve accountMe says anonymous", async () => {
    accountMe.mockResolvedValue({ authenticated: false });

    const { result } = renderHook(() => useLoginSessionGate());

    await waitFor(() => expect(result.current).toBe("anonymous"));
    expect(me).not.toHaveBeenCalled();
    expect(replace).toHaveBeenCalledWith("/login?next=%2Fdashboard");
  });
});
