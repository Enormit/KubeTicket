import { NextRequest, NextResponse } from "next/server";

export const dynamic = "force-dynamic";

async function proxyRequest(
  req: NextRequest,
  context: { params: Promise<{ path: string[] }> }
): Promise<NextResponse> {
  const { path } = await context.params;
  const backendBase = (
    process.env.BACKEND_INTERNAL_URL || "http://localhost:8080"
  ).replace(/\/+$/, "");
  const targetUrl = `${backendBase}/api/v1/${path.join("/")}${req.nextUrl.search}`;

  const forwardHeaders = new Headers();
  for (const name of [
    "accept",
    "authorization",
    "content-type",
    "cookie",
    "idempotency-key",
    "x-admission-token",
  ]) {
    const val = req.headers.get(name);
    if (val) forwardHeaders.set(name, val);
  }

  const hasBody = req.method !== "GET" && req.method !== "HEAD";
  const body = hasBody ? await req.arrayBuffer() : undefined;

  const upstream = await fetch(targetUrl, {
    method: req.method,
    headers: forwardHeaders,
    body,
    redirect: "manual",
  });

  const respHeaders = new Headers();
  const contentType = upstream.headers.get("content-type");
  if (contentType) respHeaders.set("content-type", contentType);

  const setCookies =
    typeof upstream.headers.getSetCookie === "function"
      ? upstream.headers.getSetCookie()
      : [];
  for (const cookie of setCookies) {
    respHeaders.append("set-cookie", cookie);
  }

  const respBody = await upstream.arrayBuffer();
  return new NextResponse(respBody, {
    status: upstream.status,
    headers: respHeaders,
  });
}

export const GET = proxyRequest;
export const POST = proxyRequest;
export const PUT = proxyRequest;
export const PATCH = proxyRequest;
export const DELETE = proxyRequest;
