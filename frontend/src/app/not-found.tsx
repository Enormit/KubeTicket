import Link from "next/link";
import { ArrowLeft } from "lucide-react";

export default function NotFoundPage() {
  return (
    <div className="mx-auto max-w-xl py-12">
      <section className="kt-card p-8">
        <div className="flex items-center justify-between border-b-2 border-dashed border-kt-rule pb-4 text-xs font-bold uppercase tracking-wider text-kt-muted">
          <span>MÃ LỖI CỔNG: 404</span>
          <span className="font-mono">GATE-NOT-FOUND</span>
        </div>
        <h1 className="mt-6 font-display text-5xl font-extrabold uppercase tracking-tight text-kt-ink">
          Cổng này không tồn tại
        </h1>
        <p className="mt-3 max-w-prose text-sm leading-relaxed text-kt-muted">
          Đường dẫn hoặc sự kiện bạn đang mở không có trong lịch phát hành của
          KubeTicket. Hãy quay về sảnh chính để xem các đợt mở bán đang hoạt
          động.
        </p>
        <div className="mt-8">
          <Link
            href="/"
            className="inline-flex items-center gap-2 rounded-[3px] border-2 border-kt-ink bg-kt-accent px-5 py-3 text-sm font-bold text-kt-ink transition hover:bg-kt-ink hover:text-kt-surface"
          >
            <ArrowLeft className="h-4 w-4" />
            Quay về Lịch sự kiện
          </Link>
        </div>
      </section>
    </div>
  );
}
