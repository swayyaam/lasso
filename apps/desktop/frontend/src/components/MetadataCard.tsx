import { Card, Skeleton, StatusBadge } from "@lasso/ui";
import { core } from "../bindings";
import { formatDuration } from "../format";
import { Thumbnail } from "./Thumbnail";

/**
 * MetadataCard shows what a link resolved to.
 *
 * The thumbnail frame is the `product-screenshot-card` geometry from
 * design.md: a 16px radius, aspect preserved, never cropped.
 */
export function MetadataCard({ metadata }: { metadata: core.Metadata }) {
  const playlist = metadata.kind === "playlist";
  const count = metadata.entries?.length ?? 0;

  return (
    <Card className="flex gap-md">
      <Thumbnail thumbnails={metadata.thumbnails} width={168} className="rounded-xl" />

      <div className="flex min-w-0 flex-1 flex-col gap-xxs">
        <h2 className="line-clamp-2 text-body font-medium tracking-tight text-ink" title={metadata.title}>
          {metadata.title}
        </h2>

        <p className="truncate text-caption text-ink-subtle">{metadata.uploader}</p>

        <div className="mt-auto flex flex-wrap items-center gap-xxs pt-xs">
          {playlist ? (
            <StatusBadge tone="active">{count === 1 ? "1 item" : `${count} items`}</StatusBadge>
          ) : (
            <>
              {metadata.duration > 0 && <StatusBadge>{formatDuration(metadata.duration)}</StatusBadge>}
              {/* Storyboards are preview mosaics, not something anyone can
                  download, so they are excluded from the count. */}
              {metadata.quality?.countedFormats > 0 && (
                <StatusBadge>{metadata.quality.countedFormats} formats</StatusBadge>
              )}
            </>
          )}
        </div>
      </div>
    </Card>
  );
}

/**
 * MetadataSkeleton holds the card's shape while a link resolves.
 *
 * Resolving reaches the network and takes a second or two even when warm, so
 * the wait is always visible and always the same size as the result.
 */
export function MetadataSkeleton() {
  return (
    <Card className="flex gap-md" aria-label="Resolving link">
      <Skeleton className="shrink-0 rounded-xl" style={{ width: 168, aspectRatio: "16 / 9" }} />
      <div className="flex min-w-0 flex-1 flex-col gap-xs">
        <Skeleton className="h-4 w-4/5" />
        <Skeleton className="h-3 w-1/3" />
        <div className="mt-auto flex gap-xxs pt-xs">
          <Skeleton className="h-4 w-14 rounded-pill" />
          <Skeleton className="h-4 w-20 rounded-pill" />
        </div>
      </div>
    </Card>
  );
}
