import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { logout, accountLogout, replace, loginSession, pathname } = vi.hoisted(() => ({
  logout: vi.fn().mockResolvedValue({ ok: true }),
  accountLogout: vi.fn().mockResolvedValue({ ok: true }),
  replace: vi.fn(),
  loginSession: {
    state: "authed" as const,
    surface: null as "account" | "admin" | null,
  },
  pathname: { value: "/dashboard" },
}));

vi.mock("@/lib/api", () => ({
  api: { auth: { logout, accountLogout } },
}));

vi.mock("@/lib/use-login-session-gate", () => ({
  useLoginSession: () => loginSession,
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
  usePathname: () => pathname.value,
}));

vi.mock("@/components/Logo", () => ({
  Logo: () => <div>logo</div>,
}));

vi.mock("@/components/molecules/Tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { Sidebar } from "./Sidebar";

describe("Sidebar sign-out", () => {
  beforeEach(() => {
    logout.mockClear();
    accountLogout.mockClear();
    replace.mockClear();
    loginSession.surface = null;
  });

  it("calls accountLogout when the shell is a selfserve account session", async () => {
    loginSession.surface = "account";
    render(<Sidebar />);

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(accountLogout).toHaveBeenCalledTimes(1));
    expect(logout).not.toHaveBeenCalled();
    expect(replace).toHaveBeenCalledWith("/login");
  });

  it("calls admin logout when the shell is a selfhosted admin session", async () => {
    loginSession.surface = "admin";
    render(<Sidebar />);

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(logout).toHaveBeenCalledTimes(1));
    expect(accountLogout).not.toHaveBeenCalled();
    expect(replace).toHaveBeenCalledWith("/login");
  });
});

describe("Sidebar navigation", () => {
  beforeEach(() => {
    pathname.value = "/dashboard";
  });

  it("renders the Playground nav item", () => {
    render(<Sidebar />);
    expect(screen.getByRole("link", { name: "Playground" })).toHaveAttribute(
      "href",
      "/playground",
    );
  });

  it("marks Playground active on /playground", () => {
    pathname.value = "/playground";
    render(<Sidebar />);
    expect(screen.getByRole("link", { name: "Playground" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});
