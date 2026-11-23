"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { LogOut, QrCode, ShieldCheck, Ticket } from "lucide-react";
import { useAuth } from "./AuthProvider";

export function Navbar() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();

  const handleLogout = async () => {
    await logout();
    router.push("/");
  };

  return (
    <header className="sticky top-0 z-40 border-b border-slate-800/80 bg-slate-950/90 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6">
        <Link
          href="/"
          className="flex items-center gap-2 text-lg font-bold tracking-tight text-white"
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500 text-white">
            <Ticket className="h-4 w-4" />
          </span>
          <span>KubeTicket</span>
        </Link>

        <nav className="flex items-center gap-3 text-sm">
          <Link
            href="/"
            className="rounded-md px-3 py-1.5 text-slate-300 transition hover:bg-slate-800 hover:text-white"
          >
            Sự kiện
          </Link>

          {!loading && user ? (
            <>
              <Link
                href="/tickets"
                className="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-slate-300 transition hover:bg-slate-800 hover:text-white"
              >
                <QrCode className="h-4 w-4" />
                Vé của tôi
              </Link>
              {user.role === "ADMIN" && (
                <Link
                  href="/admin"
                  className="flex items-center gap-1.5 rounded-md bg-amber-500/15 px-3 py-1.5 font-medium text-amber-300 transition hover:bg-amber-500/25"
                >
                  <ShieldCheck className="h-4 w-4" />
                  Gate Admin
                </Link>
              )}
              <span className="hidden text-xs text-slate-400 sm:inline">
                {user.full_name}
              </span>
              <button
                type="button"
                onClick={handleLogout}
                className="flex items-center gap-1 rounded-md border border-slate-700 px-3 py-1.5 text-slate-300 transition hover:border-slate-500 hover:text-white"
              >
                <LogOut className="h-3.5 w-3.5" />
                Đăng xuất
              </button>
            </>
          ) : (
            !loading && (
              <>
                <Link
                  href="/login"
                  className="rounded-md px-3 py-1.5 text-slate-300 transition hover:bg-slate-800 hover:text-white"
                >
                  Đăng nhập
                </Link>
                <Link
                  href="/register"
                  className="rounded-md bg-indigo-600 px-3.5 py-1.5 font-medium text-white transition hover:bg-indigo-500"
                >
                  Đăng ký
                </Link>
              </>
            )
          )}
        </nav>
      </div>
    </header>
  );
}
