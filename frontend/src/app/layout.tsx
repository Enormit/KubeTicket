import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/components/AuthProvider";
import { Navbar } from "@/components/Navbar";

export const metadata: Metadata = {
  title: "KubeTicket — Cloud-Native Flash Sale Ticketing",
  description:
    "Real-time Flash Sale Event Ticketing with Virtual Waiting Room, Atomic Seat Lock, and HMAC QR Check-In.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="vi">
      <body className="min-h-screen bg-[#090d16] text-slate-100 antialiased">
        <AuthProvider>
          <Navbar />
          <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6">{children}</main>
        </AuthProvider>
      </body>
    </html>
  );
}
