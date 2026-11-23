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
    setValue,
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

  const fillSeedAccount = (email: string, password: string) => {
    setValue("email", email, { shouldValidate: true });
    setValue("password", password, { shouldValidate: true });
  };

  return (
    <div className="mx-auto max-w-md py-4">
      <section className="kt-card p-6 sm:p-8">
        <h1 className="font-display text-4xl font-extrabold uppercase tracking-tight text-kt-ink">
          Đăng nhập cổng vé
        </h1>
        <p className="mt-1 text-sm text-kt-muted">
          Xác thực bằng cookie HttpOnly để vào phòng chờ và quản lý vé QR.
        </p>

        <div className="mt-5 rounded-[3px] border-2 border-dashed border-kt-rule bg-kt-ground p-3.5 text-xs">
          <p className="font-bold uppercase text-kt-ink">
            Tài khoản kiểm thử nhanh (Seed)
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() =>
                fillSeedAccount("alice@kubeticket.local", "User@123")
              }
              className="rounded-[2px] border border-kt-ink bg-kt-surface px-2.5 py-1 font-semibold text-kt-ink hover:bg-kt-alt"
            >
              Điền tài khoản Buyer
            </button>
            <button
              type="button"
              onClick={() =>
                fillSeedAccount("admin@kubeticket.local", "Admin@123")
              }
              className="rounded-[2px] border border-kt-ink bg-kt-surface px-2.5 py-1 font-semibold text-kt-ink hover:bg-kt-alt"
            >
              Điền tài khoản Admin
            </button>
          </div>
        </div>

        {serverError && (
          <div
            role="alert"
            className="mt-4 rounded-[3px] border-2 border-kt-danger bg-kt-surface p-3 text-sm font-medium text-kt-danger"
          >
            {serverError}
          </div>
        )}

        <form onSubmit={handleSubmit(onSubmit)} className="mt-6 space-y-4">
          <div>
            <label
              htmlFor="login-email"
              className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-kt-ink"
            >
              Email
            </label>
            <input
              id="login-email"
              type="email"
              {...register("email")}
              placeholder="alice@kubeticket.local"
              className="w-full rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3.5 py-2.5 text-sm text-kt-ink placeholder:text-kt-muted"
            />
            {errors.email && (
              <p className="mt-1 text-xs font-medium text-kt-danger">
                {errors.email.message}
              </p>
            )}
          </div>

          <div>
            <label
              htmlFor="login-password"
              className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-kt-ink"
            >
              Mật khẩu
            </label>
            <input
              id="login-password"
              type="password"
              {...register("password")}
              placeholder="Nhập mật khẩu"
              className="w-full rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3.5 py-2.5 text-sm text-kt-ink placeholder:text-kt-muted"
            />
            {errors.password && (
              <p className="mt-1 text-xs font-medium text-kt-danger">
                {errors.password.message}
              </p>
            )}
          </div>

          <button
            type="submit"
            disabled={isSubmitting}
            className="w-full rounded-[3px] border-2 border-kt-ink bg-kt-accent py-3 text-sm font-extrabold uppercase tracking-wide text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface disabled:opacity-60"
          >
            {isSubmitting ? "Đang xác thực..." : "Đăng nhập ngay"}
          </button>
        </form>

        <p className="mt-6 border-t-2 border-dashed border-kt-rule pt-4 text-center text-sm text-kt-muted">
          Chưa có tài khoản?{" "}
          <Link
            href="/register"
            className="font-bold text-kt-ink underline underline-offset-4 hover:text-kt-accent"
          >
            Đăng ký tại đây
          </Link>
        </p>
      </section>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-md kt-card p-8 text-sm text-kt-muted">
          Đang tải biểu mẫu đăng nhập...
        </div>
      }
    >
      <LoginFormInner />
    </Suspense>
  );
}
