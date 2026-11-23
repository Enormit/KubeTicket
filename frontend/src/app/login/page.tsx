"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { api, APIError } from "@/lib/api";
import { broadcastTabSync } from "@/lib/multi-tab";
import { useAuth } from "@/components/AuthProvider";

const loginSchema = z.object({
  email: z.string().email("Email không hợp lệ"),
  password: z.string().min(6, "Mật khẩu tối thiểu 6 ký tự"),
});

type LoginForm = z.infer<typeof loginSchema>;

function LoginFormInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextPath = searchParams.get("next") || "/";
  const { refresh } = useAuth();
  const [serverError, setServerError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
  });

  const onSubmit = async (values: LoginForm) => {
    setServerError(null);
    try {
      await api.auth.login(values.email, values.password);
      await refresh();
      broadcastTabSync({ type: "AUTH_CHANGED" });
      router.push(nextPath);
    } catch (err) {
      setServerError(
        err instanceof APIError ? err.message : "Đăng nhập thất bại"
      );
    }
  };

  return (
    <div className="mx-auto max-w-md rounded-2xl border border-slate-800 bg-slate-900/80 p-8 shadow-xl">
      <h1 className="text-2xl font-bold text-white">Đăng nhập KubeTicket</h1>
      <p className="mt-1 text-sm text-slate-400">
        Phiên đăng nhập được bảo vệ bằng cookie HttpOnly.
      </p>

      <div className="mt-4 rounded-lg border border-slate-800 bg-slate-950/80 p-3 text-xs text-slate-400">
        <p className="font-semibold text-slate-300">Tài khoản test (Seed):</p>
        <p>User: alice@kubeticket.local / User@123</p>
        <p>Admin: admin@kubeticket.local / Admin@123</p>
      </div>

      {serverError && (
        <div className="mt-4 rounded-lg border border-rose-500/40 bg-rose-950/40 p-3 text-sm text-rose-200">
          {serverError}
        </div>
      )}

      <form onSubmit={handleSubmit(onSubmit)} className="mt-6 space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-300">
            Email
          </label>
          <input
            type="email"
            {...register("email")}
            placeholder="alice@kubeticket.local"
            className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
          />
          {errors.email && (
            <p className="mt-1 text-xs text-rose-400">{errors.email.message}</p>
          )}
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-slate-300">
            Mật khẩu
          </label>
          <input
            type="password"
            {...register("password")}
            placeholder="••••••••"
            className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
          />
          {errors.password && (
            <p className="mt-1 text-xs text-rose-400">
              {errors.password.message}
            </p>
          )}
        </div>

        <button
          type="submit"
          disabled={isSubmitting}
          className="w-full rounded-lg bg-indigo-600 py-2.5 text-sm font-semibold text-white transition hover:bg-indigo-500 disabled:opacity-60"
        >
          {isSubmitting ? "Đang đăng nhập..." : "Đăng nhập"}
        </button>
      </form>

      <p className="mt-6 text-center text-sm text-slate-400">
        Chưa có tài khoản?{" "}
        <Link href="/register" className="text-indigo-400 hover:underline">
          Đăng ký ngay
        </Link>
      </p>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-md rounded-2xl border border-slate-800 bg-slate-900/80 p-8 text-sm text-slate-400">
          Đang tải...
        </div>
      }
    >
      <LoginFormInner />
    </Suspense>
  );
}
