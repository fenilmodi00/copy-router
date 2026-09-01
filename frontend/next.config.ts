import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";
const ROUTER_DEV_TARGET = process.env.ROUTER_DEV_TARGET ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  // Static export for production; dev server uses local .next so the runtime
  // can resolve next/* modules from frontend/node_modules.
  ...(isDev
    ? {
        async rewrites() {
          // Proxy dashboard API calls to the Go router. `/v1` is listed
          // twice: origin-root fetch("/v1/...") (basePath: false) and
          // /ui/v1/* (Next basePath), which otherwise renders as a page 404.
          return [
            {
              source: "/v1/:path*",
              destination: `${ROUTER_DEV_TARGET}/v1/:path*`,
              basePath: false,
            },
            {
              source: "/v1/:path*",
              destination: `${ROUTER_DEV_TARGET}/v1/:path*`,
            },
            {
              source: "/account/:path*",
              destination: `${ROUTER_DEV_TARGET}/account/:path*`,
              basePath: false,
            },
          ];
        },
        // Bare-root convenience redirect so visiting localhost:3000
        // lands on the dashboard. In production the Go server does
        // this; in `next dev` requests hit Next directly and never
        // reach the Go backend, so we replicate it here.
        async redirects() {
          return [
            {
              source: "/",
              destination: "/ui/",
              basePath: false,
              permanent: false,
            },
          ];
        },
      }
    : {
        // Static export → frontend/out/ (Next.js default). The Dockerfile
        // copies that into the Go server's assets/ui at the next stage.
        // Keep distDir at its default (`.next`) so we don't write outside
        // the project directory, which newer Next.js versions reject.
        output: "export",
      }),
  basePath: "/ui",
  devIndicators: false,
  images: { unoptimized: true },
  trailingSlash: false,
};

export default nextConfig;
