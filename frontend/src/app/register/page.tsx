"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { api, APIError } from "@/lib/api";
import { broadcastTabSync } from "@/lib/multi-tab";
import { useAuth } from "@/components/AuthProvider";

const registerSchema = z.object({
  full_name: z.string().min(2, "Vui lòng nhập họ tên"),
  email: z.string().email("Email không hợp lệ"),
  password: z.string().min(6, "Mật khẩu tối thiểu 6 ký tự"),
});

type RegisterForm = z.infer<typeof registerSchema>;

export default function RegisterPage() {
  const router = useRouter();
  const { refresh } = useAuth();
  const [serverError, setServerError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterForm>({
    resolver: zodResolver(registerSchema),
  });

  const onSubmit = async (values: RegisterForm) => {
    setServerError(null);
    try {
      await api.auth.register(values.email, values.password, values.full_name);
      await refresh();
      broadcastTabSync({ type: "AUTH_CHANGED" });
      router.push("/");
    } catch (err) {
      setServerError(
        err instanceof APIError ? err.message : "Đăng ký thất bại"
      );
    }
  };

  return (
    <div className="mx-auto max-w-md rounded-2xl border border-slate-800 bg-slate-900/80 p-8 shadow-xl">
      <h1 className="text-2xl font-bold text-white">Tạo tài khoản KubeTicket</h1>
      <p className="mt-1 text-sm text-slate-400">
        Đăng ký nhanh để tham gia hàng chờ ảo và đặt vé.
      </p>

      {serverError && (
        <div className="mt-4 rounded-lg border border-rose-500/40 bg-rose-950/40 p-3 text-sm text-rose-200">
          {serverError}
        </div>
      )}

      <form onSubmit={handleSubmit(onSubmit)} className="mt-6 space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-300">
            Họ và tên
          </label>
          <input
            type="text"
            {...register("full_name")}
            placeholder="Nguyen Van A"
            className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
          />
          {errors.full_name && (
            <p className="mt-1 text-xs text-rose-400">
              {errors.full_name.message}
            </p>
          )}
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-slate-300">
            Email
          </label>
          <input
            type="email"
            {...register("email")}
            placeholder="you@example.com"
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
            placeholder="Tối thiểu 6 ký tự"
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
          {isSubmitting ? "Đang tạo tài khoản..." : "Đăng ký"}
        </button>
      </form>

      <p className="mt-6 text-center text-sm text-slate-400">
        Đã có tài khoản?{" "}
        <Link href="/login" className="text-indigo-400 hover:underline">
          Đăng nhập
        </Link>
      </p>
    </div>
  );
}
