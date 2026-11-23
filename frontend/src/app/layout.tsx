import type { Metadata } from "next";
import {
  Barlow_Condensed,
  Be_Vietnam_Pro,
  JetBrains_Mono,
} from "next/font/google";
import "./globals.css";
import { AuthProvider } from "@/components/AuthProvider";
import { Navbar } from "@/components/Navbar";

const barlowCondensed = Barlow_Condensed({
  subsets: ["latin", "vietnamese"],
  weight: ["700", "800"],
  variable: "--font-display",
  display: "swap",
});

const beVietnamPro = Be_Vietnam_Pro({
  subsets: ["latin", "vietnamese"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-sans",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin", "vietnamese"],
  weight: ["500", "700"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "KubeTicket — Phòng vé Sân vận động & Flash Sale Thời gian thực",
  description:
    "Hệ thống bán vé Flash Sale đảm bảo Zero Overselling với Phòng chờ ảo FIFO, Khóa ghế Redis Lua và Vé điện tử QR HMAC-SHA256.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="vi"
      className={`${barlowCondensed.variable} ${beVietnamPro.variable} ${jetbrainsMono.variable}`}
    >
      <body className="min-h-screen bg-kt-ground font-sans text-kt-ink antialiased">
        <AuthProvider>
          <Navbar />
          <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:py-8">
            {children}
          </main>
          <footer className="mt-16 border-t-2 border-kt-ink bg-kt-surface">
            <div className="mx-auto flex max-w-7xl flex-col gap-3 px-4 py-6 text-xs text-kt-muted sm:flex-row sm:items-center sm:justify-between sm:px-6">
              <div className="flex items-center gap-3 font-medium text-kt-ink">
                <span className="font-display text-base font-extrabold tracking-tight uppercase">
                  KubeTicket Box Office
                </span>
                <span>•</span>
                <span>Redis Lua Atomic Lock &amp; PostgreSQL ACID</span>
              </div>
              <p>
                Dữ liệu sự kiện mẫu dùng cho trình diễn đặt vé trực tuyến &amp;
                kiểm thử tải đồng thời.
              </p>
            </div>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
