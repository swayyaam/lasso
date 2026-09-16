import { useCallback, useEffect, useRef, useState } from "react";
import { api, binaries, core, EventsOff, EventsOn, main, presets } from "../bindings";

/**
 * useEventNames fetches the backend's event names once.
 *
 * The names live in Go and are handed over at runtime rather than duplicated
 * as string literals here, so renaming an event cannot leave the frontend
 * silently subscribed to nothing.
 */
function useEventNames() {
  const [names, setNames] = useState<main.EventNames | null>(null);

  useEffect(() => {
    let cancelled = false;
    api.Events().then((n) => {
      if (!cancelled) setNames(n);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return names;
}

/**
 * useQueue keeps a live copy of the download queue.
 *
 * State changes carry the whole item and are rare; progress is frequent and
 * carries only what moved. Progress is already throttled per item in the
 * backend, so there is nothing to debounce here.
 */
export function useQueue() {
  const [items, setItems] = useState<core.Item[]>([]);
  const names = useEventNames();

  useEffect(() => {
    if (!names) return;
    let cancelled = false;

    api.QueueItems().then((initial) => {
      if (!cancelled) setItems(initial);
    });

    EventsOn(names.queueItem, (item: core.Item) => {
      setItems((current) => {
        const at = current.findIndex((i) => i.id === item.id);
        if (at === -1) return [...current, item];
        const next = current.slice();
        next[at] = item;
        return next;
      });
    });

    EventsOn(names.queueProgress, (event: { id: string; progress: core.Progress }) => {
      setItems((current) => {
        const at = current.findIndex((i) => i.id === event.id);
        if (at === -1) return current;
        const next = current.slice();
        next[at] = { ...next[at], progress: event.progress } as core.Item;
        return next;
      });
    });

    return () => {
      cancelled = true;
      EventsOff(names.queueItem);
      EventsOff(names.queueProgress);
    };
  }, [names]);

  return items;
}

/** useBinaryStatus tracks whether the helper programs are usable. */
export function useBinaryStatus() {
  const [status, setStatus] = useState<binaries.Status | null>(null);
  const names = useEventNames();

  useEffect(() => {
    if (!names) return;
    let cancelled = false;

    api.BinaryStatus().then((s) => {
      if (!cancelled) setStatus(s);
    });
    EventsOn(names.binaryStatus, (s: binaries.Status) => setStatus(s));

    return () => {
      cancelled = true;
      EventsOff(names.binaryStatus);
    };
  }, [names]);

  return status;
}

/** useSettings exposes the preferences and a saver that reports failures. */
export function useSettings() {
  const [settings, setSettings] = useState<main.Settings | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api.Settings().then((s) => {
      if (!cancelled) setSettings(s);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const save = useCallback(async (next: main.Settings) => {
    setError("");
    try {
      // The backend normalises and returns what it actually applied, so the
      // form reflects reality rather than what was asked for.
      setSettings(await api.SaveSettings(next));
      return true;
    } catch (e) {
      setError(String(e));
      return false;
    }
  }, []);

  return { settings, save, error };
}

/** usePresets exposes the preset list and keeps it fresh after every change. */
export function usePresets() {
  const [items, setItems] = useState<presets.Preset[] | null>(null);

  const refresh = useCallback(async () => {
    setItems(await api.Presets());
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { presets: items, refresh };
}

/**
 * useLatest keeps a ref pointing at the newest value, so an event listener
 * registered once can read current state without being torn down and
 * re-registered on every keystroke.
 */
export function useLatest<T>(value: T) {
  const ref = useRef(value);
  ref.current = value;
  return ref;
}
