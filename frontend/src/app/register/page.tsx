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
  full_name: z.string().min(2, "Vui lòng nhập họ tên đầy đủ"),
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
    <div className="mx-auto max-w-md py-4">
      <section className="kt-card p-6 sm:p-8">
        <h1 className="font-display text-4xl font-extrabold uppercase tracking-tight text-kt-ink">
          Tạo thẻ thành viên
        </h1>
        <p className="mt-1 text-sm text-kt-muted">
          Đăng ký tài khoản để vào hàng chờ ảo và lưu vé điện tử QR.
        </p>

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
              htmlFor="reg-fullname"
              className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-kt-ink"
            >
              Họ và tên
            </label>
            <input
              id="reg-fullname"
              type="text"
              {...register("full_name")}
              placeholder="Nguyễn Văn A"
              className="w-full rounded-[3px] border-2 border-kt-ink bg-kt-ground px-3.5 py-2.5 text-sm text-kt-ink placeholder:text-kt-muted"
            />
            {errors.full_name && (
              <p className="mt-1 text-xs font-medium text-kt-danger">
                {errors.full_name.message}
              </p>
            )}
          </div>

          <div>
            <label
              htmlFor="reg-email"
              className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-kt-ink"
            >
              Email
            </label>
            <input
              id="reg-email"
              type="email"
              {...register("email")}
              placeholder="ban@example.com"
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
              htmlFor="reg-password"
              className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-kt-ink"
            >
              Mật khẩu
            </label>
            <input
              id="reg-password"
              type="password"
              {...register("password")}
              placeholder="Tối thiểu 6 ký tự"
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
            {isSubmitting ? "Đang tạo tài khoản..." : "Hoàn tất đăng ký"}
          </button>
        </form>

        <p className="mt-6 border-t-2 border-dashed border-kt-rule pt-4 text-center text-sm text-kt-muted">
          Đã có tài khoản?{" "}
          <Link
            href="/login"
            className="font-bold text-kt-ink underline underline-offset-4 hover:text-kt-accent"
          >
            Đăng nhập
          </Link>
        </p>
      </section>
    </div>
  );
}
