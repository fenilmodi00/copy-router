import Link from "next/link";

interface LogoProps {
  href?: string;
}

// basePath is "/ui" (see next.config.ts). Hardcoded so it works in both
// next dev (where next/image's basePath rewrite isn't reliable with our
// unoptimized + dev-rewrites combo) and the static /ui export.
const LOGO_SRC = "/ui/aiand.svg";

export function Logo({ href = "/" }: LogoProps) {
  return (
    <Link href={href} className="inline-flex shrink-0" aria-label="aiand-auto">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src={LOGO_SRC} alt="aiand-auto" width={29} height={32} className="h-8 w-auto shrink-0 grow-0" />
    </Link>
  );
}
