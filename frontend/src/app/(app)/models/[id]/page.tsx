// The router dashboard ships as a static export (next.config.ts), which
// requires every dynamic segment to be enumerated at build time. Detail pages
// are fully client-rendered and the catalog is per-install, so the real ids
// can't be known here — emit one placeholder and let view.tsx resolve the
// actual id from the URL at runtime (client-side nav via <Link>/router.push
// always carries real params).
export function generateStaticParams() {
  return [{ id: "__none__" }];
}

import ModelDetailView from "./view";

export default async function ModelDetailPage(props: { params?: Promise<{ id?: string }> }) {
  const { id } = (await props.params) ?? {};
  return <ModelDetailView params={id !== undefined ? { id } : undefined} />;
}