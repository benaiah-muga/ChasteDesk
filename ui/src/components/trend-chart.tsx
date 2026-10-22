// SPDX-License-Identifier: AGPL-3.0-only
import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import { shortDate } from "@/lib/format";

export interface TrendPoint {
  date: string;
  total: number;
}

function linePath(points: Array<{ x: number; y: number }>): string {
  if (points.length === 0) return "";
  if (points.length === 1) return `M ${points[0].x} ${points[0].y}`;
  let path = `M ${points[0].x} ${points[0].y}`;
  for (let index = 1; index < points.length; index += 1) {
    const previous = points[index - 1];
    const current = points[index];
    const midX = (previous.x + current.x) / 2;
    path += ` C ${midX} ${previous.y}, ${midX} ${current.y}, ${current.x} ${current.y}`;
  }
  return path;
}

export function TrendChart({
  data,
  height = 220,
  className,
  label = "Conversations per day",
}: {
  data: TrendPoint[];
  height?: number;
  className?: string;
  label?: string;
}) {
  const [active, setActive] = useState<number | null>(null);
  const width = 760;
  const padding = { top: 18, right: 14, bottom: 30, left: 34 };
  const max = Math.max(1, ...data.map((point) => point.total));
  const min = 0;
  const plotWidth = width - padding.left - padding.right;
  const plotHeight = height - padding.top - padding.bottom;
  const points = useMemo(
    () =>
      data.map((point, index) => ({
        x: padding.left + (data.length === 1 ? plotWidth / 2 : (index / (data.length - 1)) * plotWidth),
        y: padding.top + plotHeight - ((point.total - min) / (max - min)) * plotHeight,
        point,
      })),
    [data, max, plotHeight, plotWidth, padding.left, padding.top],
  );
  const line = linePath(points);
  const area = points.length
    ? `${line} L ${points[points.length - 1].x} ${padding.top + plotHeight} L ${points[0].x} ${padding.top + plotHeight} Z`
    : "";
  const yTicks = [0, Math.round(max / 2), max];
  const activePoint = active === null ? null : points[active] ?? null;
  if (data.length === 0) {
    return <div className="flex h-44 items-center justify-center rounded-lg border border-dashed border-rule text-[12px] text-muted-foreground">No daily data yet.</div>;
  }
  return (
    <div className={cn("relative w-full", className)} role="img" aria-label={label}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        className="w-full overflow-visible"
        style={{ height }}
        onMouseLeave={() => setActive(null)}
      >
        {yTicks.map((tick) => {
          const y = padding.top + plotHeight - ((tick - min) / (max - min)) * plotHeight;
          return (
            <g key={tick}>
              <line x1={padding.left} x2={width - padding.right} y1={y} y2={y} stroke="var(--color-rule)" strokeWidth="1" strokeDasharray={tick === 0 ? undefined : "2 5"} />
              <text x={padding.left - 9} y={y + 3} textAnchor="end" fill="var(--color-faint)" fontSize="9" fontFamily="var(--font-mono)">
                {tick}
              </text>
            </g>
          );
        })}
        <path d={area} fill="var(--color-accent-soft)" opacity="0.7" />
        <path d={line} fill="none" stroke="var(--color-accent-strong)" strokeWidth="1.75" strokeLinecap="round" />
        {points.map((point, index) => (
          <g key={`${point.point.date}-${index}`}>
            <rect
              x={point.x - Math.max(8, plotWidth / Math.max(data.length, 1) / 2)}
              y={padding.top}
              width={Math.max(16, plotWidth / Math.max(data.length, 1))}
              height={plotHeight}
              fill="transparent"
              onMouseEnter={() => setActive(index)}
              onFocus={() => setActive(index)}
              tabIndex={-1}
            />
            {active === index ? (
              <g>
                <line x1={point.x} x2={point.x} y1={padding.top} y2={padding.top + plotHeight} stroke="var(--color-rule-strong)" strokeDasharray="2 4" />
                <circle cx={point.x} cy={point.y} r="4.5" fill="var(--color-surface)" stroke="var(--color-accent-strong)" strokeWidth="2" />
              </g>
            ) : null}
          </g>
        ))}
        {[0, Math.floor((data.length - 1) / 2), data.length - 1].filter((value, index, arr) => arr.indexOf(value) === index).map((index) => {
          const point = points[index];
          if (!point) return null;
          return (
            <text
              key={index}
              x={point.x}
              y={height - 8}
              textAnchor={index === 0 ? "start" : index === data.length - 1 ? "end" : "middle"}
              fill="var(--color-faint)"
              fontSize="9"
              fontFamily="var(--font-mono)"
            >
              {shortDate(point.point.date)}
            </text>
          );
        })}
      </svg>
      {activePoint ? (
        <div
          className="pointer-events-none absolute z-10 -translate-x-1/2 rounded-md border border-rule bg-surface px-2.5 py-1.5 text-[11px] shadow-[var(--shadow-float)]"
          style={{ left: `${(activePoint.x / width) * 100}%`, top: `${Math.max(0, (activePoint.y / height) * height - 44)}px` }}
        >
          <span className="font-mono text-muted-foreground">{shortDate(activePoint.point.date)}</span>
          <span className="ml-2 font-semibold tabular">{activePoint.point.total}</span>
        </div>
      ) : null}
    </div>
  );
}

export function Sparkline({ data, className }: { data: TrendPoint[]; className?: string }) {
  const width = 150;
  const height = 38;
  const max = Math.max(1, ...data.map((point) => point.total));
  const points = data.map((point, index) => ({
    x: data.length === 1 ? width / 2 : (index / (data.length - 1)) * width,
    y: height - 4 - (point.total / max) * (height - 8),
  }));
  const path = linePath(points);
  if (data.length === 0) return null;
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className={cn("h-9 w-[150px] overflow-visible", className)} aria-hidden="true">
      <path d={`${path} L ${width} ${height} L 0 ${height} Z`} fill="var(--color-accent-soft)" opacity="0.55" />
      <path d={path} fill="none" stroke="var(--color-accent-strong)" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}
