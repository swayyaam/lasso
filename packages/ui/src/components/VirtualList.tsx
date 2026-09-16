import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { cx } from "../cx";

/**
 * VirtualList renders only the rows that are on screen.
 *
 * A playlist can put hundreds of items in the queue, each with its own
 * progress bar and thumbnail, and mounting all of them makes scrolling stutter
 * and every progress event re-render work nobody can see. Rows are a fixed
 * height, which is what lets the window be computed arithmetically instead of
 * measured.
 *
 * Deliberately hand-written rather than pulled from a library: fixed-height
 * windowing is a small, well-understood problem, and packages/core's habit of
 * staying dependency-light is worth keeping here too.
 */
export function VirtualList<T>({
  items,
  rowHeight,
  renderRow,
  getKey,
  overscan = 6,
  className,
  empty,
}: {
  items: T[];
  /** Every row must be exactly this tall, including its own margins. */
  rowHeight: number;
  renderRow: (item: T, index: number) => ReactNode;
  getKey: (item: T, index: number) => string;
  /** Rows rendered beyond each edge, so fast scrolling does not show gaps. */
  overscan?: number;
  className?: string;
  empty?: ReactNode;
}) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  const measure = useCallback(() => {
    const el = viewportRef.current;
    if (el) setViewportHeight(el.clientHeight);
  }, []);

  useLayoutEffect(measure, [measure]);

  useEffect(() => {
    const el = viewportRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [measure]);

  // Until the viewport has been measured, render a first screenful rather than
  // nothing, so the initial paint is not empty.
  const effectiveHeight = viewportHeight || rowHeight * 8;

  const first = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const visibleCount = Math.ceil(effectiveHeight / rowHeight) + overscan * 2;
  const last = Math.min(items.length, first + visibleCount);

  const window = items.slice(first, last);

  if (items.length === 0 && empty) {
    return <div className={cx("overflow-y-auto", className)}>{empty}</div>;
  }

  return (
    <div
      ref={viewportRef}
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
      className={cx("overflow-y-auto", className)}
    >
      {/* A single spacer of the full height gives the scrollbar its correct
          size and position without mounting the rows it represents. */}
      <div style={{ height: items.length * rowHeight, position: "relative" }}>
        <div style={{ transform: `translateY(${first * rowHeight}px)` }}>
          {window.map((item, i) => (
            <div key={getKey(item, first + i)} style={{ height: rowHeight }}>
              {renderRow(item, first + i)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
