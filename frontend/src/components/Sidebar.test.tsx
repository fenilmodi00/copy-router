import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { accountLogout, replace, loginSession, pathname } = vi.hoisted(() => ({
  accountLogout: vi.fn().mockResolvedValue({ ok: true }),
  replace: vi.fn(),
  loginSession: {
    state: "authed" as const,
    surface: null as "account" | null,
  },
  pathname: { value: "/dashboard" },
}));

vi.mock("@/lib/api", () => ({
  api: { auth: { accountLogout } },
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
    accountLogout.mockClear();
    replace.mockClear();
    loginSession.surface = null;
  });

  it("calls accountLogout when the shell is a selfserve account session", async () => {
    loginSession.surface = "account";
    render(<Sidebar />);

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(accountLogout).toHaveBeenCalledTimes(1));
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
