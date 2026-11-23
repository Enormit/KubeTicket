"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { LogOut, Menu, QrCode, ShieldCheck, X } from "lucide-react";
import { useAuth } from "./AuthProvider";

export function Navbar() {
  const { user, loading, logout } = useAuth();
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);

  const handleLogout = async () => {
    await logout();
    setMobileOpen(false);
    router.push("/");
  };

  const navLinkClass = (href: string) => {
    const active = href === "/" ? pathname === "/" : pathname.startsWith(href);
    return `px-2.5 py-1.5 text-sm font-semibold transition-colors ${
      active
        ? "bg-kt-ink text-kt-surface rounded-[3px]"
        : "text-kt-ink hover:bg-kt-alt rounded-[3px]"
    }`;
  };

  return (
    <header className="sticky top-0 z-40 border-b-2 border-kt-ink bg-kt-surface">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6">
        <Link
          href="/"
          className="flex items-center gap-2.5 text-kt-ink"
          aria-label="KubeTicket Trang chủ"
        >
          <svg
            width="28"
            height="20"
            viewBox="0 0 28 20"
            fill="none"
            aria-hidden="true"
            className="shrink-0"
          >
            <path
              d="M2 2H26V7.2C24.5 7.2 23.3 8.4 23.3 10C23.3 11.6 24.5 12.8 26 12.8V18H2V12.8C3.5 12.8 4.7 11.6 4.7 10C4.7 8.4 3.5 7.2 2 7.2V2Z"
              fill="#E85D04"
              stroke="#121212"
              strokeWidth="2"
            />
            <line
              x1="10"
              y1="3"
              x2="10"
              y2="17"
              stroke="#121212"
              strokeWidth="1.5"
              strokeDasharray="2 2"
            />
          </svg>
          <span className="font-display text-2xl font-extrabold tracking-tight uppercase">
            KubeTicket
          </span>
        </Link>

        {/* Desktop nav */}
        <nav
          aria-label="Điều hướng chính"
          className="hidden items-center gap-2 md:flex"
        >
          <Link href="/" className={navLinkClass("/")}>
            Lịch sự kiện
          </Link>

          {!loading && user ? (
            <>
              <Link href="/tickets" className={navLinkClass("/tickets")}>
                <span className="inline-flex items-center gap-1.5">
                  <QrCode className="h-4 w-4" />
                  Vé của tôi
                </span>
              </Link>
              {user.role === "ADMIN" && (
                <Link href="/admin" className={navLinkClass("/admin")}>
                  <span className="inline-flex items-center gap-1.5">
                    <ShieldCheck className="h-4 w-4" />
                    Cổng soát vé
                  </span>
                </Link>
              )}
              <span className="mx-1 h-4 w-px bg-kt-rule" aria-hidden="true" />
              <span className="text-xs font-medium text-kt-muted">
                {user.full_name}
              </span>
              <button
                type="button"
                onClick={handleLogout}
                className="inline-flex items-center gap-1.5 rounded-[3px] border-2 border-kt-ink bg-kt-surface px-3 py-1.5 text-xs font-bold text-kt-ink transition hover:bg-kt-alt"
              >
                <LogOut className="h-3.5 w-3.5" />
                Đăng xuất
              </button>
            </>
          ) : (
            !loading && (
              <>
                <Link href="/login" className={navLinkClass("/login")}>
                  Đăng nhập
                </Link>
                <Link
                  href="/register"
                  className="rounded-[3px] border-2 border-kt-ink bg-kt-accent px-3.5 py-1.5 text-sm font-bold text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface"
                >
                  Đăng ký
                </Link>
              </>
            )
          )}
        </nav>

        {/* Mobile menu button */}
        <button
          type="button"
          onClick={() => setMobileOpen((v) => !v)}
          aria-label={mobileOpen ? "Đóng menu" : "Mở menu"}
          aria-expanded={mobileOpen}
          className="inline-flex items-center justify-center rounded-[3px] border-2 border-kt-ink p-2 text-kt-ink md:hidden"
        >
          {mobileOpen ? (
            <X className="h-5 w-5" />
          ) : (
            <Menu className="h-5 w-5" />
          )}
        </button>
      </div>

      {/* Mobile drawer */}
      {mobileOpen && (
        <nav
          aria-label="Điều hướng di động"
          className="flex flex-col gap-2 border-t-2 border-kt-ink bg-kt-surface px-4 py-4 md:hidden"
        >
          <Link
            href="/"
            onClick={() => setMobileOpen(false)}
            className={navLinkClass("/")}
          >
            Lịch sự kiện
          </Link>
          {!loading && user ? (
            <>
              <Link
                href="/tickets"
                onClick={() => setMobileOpen(false)}
                className={navLinkClass("/tickets")}
              >
                Vé của tôi
              </Link>
              {user.role === "ADMIN" && (
                <Link
                  href="/admin"
                  onClick={() => setMobileOpen(false)}
                  className={navLinkClass("/admin")}
                >
                  Cổng soát vé
                </Link>
              )}
              <button
                type="button"
                onClick={handleLogout}
                className="mt-1 flex items-center justify-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-alt py-2 text-sm font-bold text-kt-ink"
              >
                <LogOut className="h-4 w-4" />
                Đăng xuất ({user.full_name})
              </button>
            </>
          ) : (
            !loading && (
              <div className="grid grid-cols-2 gap-2 pt-1">
                <Link
                  href="/login"
                  onClick={() => setMobileOpen(false)}
                  className="rounded-[3px] border-2 border-kt-ink py-2 text-center text-sm font-bold text-kt-ink"
                >
                  Đăng nhập
                </Link>
                <Link
                  href="/register"
                  onClick={() => setMobileOpen(false)}
                  className="rounded-[3px] border-2 border-kt-ink bg-kt-accent py-2 text-center text-sm font-bold text-kt-ink"
                >
                  Đăng ký
                </Link>
              </div>
            )
          )}
        </nav>
      )}
    </header>
  );
}
