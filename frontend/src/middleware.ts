import { NextRequest, NextResponse } from "next/server";

const PROTECTED_PREFIXES = [
  "/waiting-room",
  "/checkout",
  "/tickets",
  "/admin",
];

export function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl;
  const isProtected =
    PROTECTED_PREFIXES.some((prefix) => pathname.startsWith(prefix)) ||
    /^\/events\/[^/]+\/seats/.test(pathname);

  if (!isProtected) {
    return NextResponse.next();
  }

  const token = req.cookies.get("kubeticket_token")?.value;
  if (!token) {
    const loginUrl = new URL("/login", req.url);
    loginUrl.searchParams.set("next", `${pathname}${search}`);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    "/waiting-room/:path*",
    "/events/:path*/seats",
    "/checkout/:path*",
    "/tickets/:path*",
    "/admin/:path*",
  ],
};
