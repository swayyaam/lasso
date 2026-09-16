import { useEffect, useRef, useState } from "react";
import { api, core, EventsOn, EventsOff, main } from "./bindings";

/**
 * useQueue keeps a live copy of the download queue.
 *
 * State changes and progress arrive as separate events: state changes are rare
 * and carry the whole item, while progress is frequent and carries only what
 * moved. Progress is already throttled in the backend to one update per item
 * per window, so this does not need to debounce again.
 */
export function useQueue() {
  const [items, setItems] = useState<core.Item[]>([]);
  const namesRef = useRef<main.EventNames | null>(null);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      const names = await api.Events();
      if (cancelled) return;
      namesRef.current = names;

      setItems(await api.QueueItems());

      EventsOn(names.queueItem, (item: core.Item) => {
        setItems((current) => {
          const next = current.slice();
          const at = next.findIndex((i) => i.id === item.id);
          if (at === -1) next.push(item);
          else next[at] = item;
          return next;
        });
      });

      EventsOn(names.queueProgress, (event: { id: string; progress: core.Progress }) => {
        setItems((current) =>
          current.map((item) =>
            item.id === event.id ? { ...item, progress: event.progress } as core.Item : item,
          ),
        );
      });
    })();

    return () => {
      cancelled = true;
      const names = namesRef.current;
      if (names) {
        EventsOff(names.queueItem);
        EventsOff(names.queueProgress);
      }
    };
  }, []);

  return items;
}

/** useBinaryStatus tracks whether the sidecar binaries are usable. */
export function useBinaryStatus() {
  const [status, setStatus] = useState<binariesStatus | null>(null);

  useEffect(() => {
    let cancelled = false;
    let eventName = "";

    (async () => {
      const names = await api.Events();
      if (cancelled) return;
      eventName = names.binaryStatus;
      setStatus(await api.BinaryStatus());
      EventsOn(eventName, (next: binariesStatus) => setStatus(next));
    })();

    return () => {
      cancelled = true;
      if (eventName) EventsOff(eventName);
    };
  }, []);

  return status;
}

// Imported lazily to avoid a circular import in the generated models.
type binariesStatus = Awaited<ReturnType<typeof api.BinaryStatus>>;
