import { cn } from "@/lib/cn";

// Dependency-free SVG sparkline for the KPI cards. Scales the polyline to the
// data min/max; a flat or single-point series renders a dashed neutral line.
export function Sparkline({
  data,
  width = 120,
  height = 32,
  strokeClass = "stroke-primary",
  fillClass = "fill-primary/10",
}: {
  data: number[];
  width?: number;
  height?: number;
  strokeClass?: string;
  fillClass?: string;
}) {
  if (data.length < 2) {
    return (
      <svg width={width} height={height} className="block" aria-hidden>
        <line
          x1={0}
          y1={height / 2}
          x2={width}
          y2={height / 2}
          className={cn("stroke-muted-foreground/30", strokeClass)}
          strokeWidth={1.5}
          strokeDasharray="3 3"
        />
      </svg>
    );
  }
  const min = Math.min(...data);
  const max = Math.max(...data);
  const span = max - min || 1;
  const stepX = width / (data.length - 1);
  const pts = data.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / span) * (height - 4) - 2;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const area = `M0,${height} L${pts.join(" L")} L${width},${height} Z`;
  return (
    <svg width={width} height={height} className="block" aria-hidden>
      <polyline points={pts.join(" ")} className={cn("fill-none", strokeClass)} strokeWidth={1.5} />
      <path d={area} className={fillClass} />
    </svg>
  );
}