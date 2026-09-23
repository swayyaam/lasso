import { useCallback, useEffect, useRef, useState } from "react";
import { api, binaries, core, EventsOff, EventsOn, history, main, presets } from "../bindings";

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

    // Removal is the one change that cannot arrive as an item: there is no
    // item left to send, only the ids that went.
    EventsOn(names.queueRemoved, (ids: string[]) => {
      const gone = new Set(ids);
      setItems((current) => current.filter((i) => !gone.has(i.id)));
    });

    return () => {
      cancelled = true;
      EventsOff(names.queueItem);
      EventsOff(names.queueProgress);
      EventsOff(names.queueRemoved);
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
 * useHistory keeps the record of finished downloads in step with the backend.
 *
 * The change event carries nothing and the list is fetched in response, so
 * there is one definition of what history is rather than a copy maintained by
 * replaying events into local state.
 */
export function useHistory() {
  const [entries, setEntries] = useState<history.Entry[] | null>(null);
  const names = useEventNames();

  const refresh = useCallback(async () => {
    try {
      // Null means "not loaded yet" and renders nothing, so a failure must not
      // leave it there: an empty list at least explains itself.
      setEntries((await api.History()) ?? []);
    } catch {
      setEntries([]);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!names) return;
    EventsOn(names.historyChanged, () => void refresh());
    return () => EventsOff(names.historyChanged);
  }, [names, refresh]);

  return { entries, refresh };
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

/** useCommandNames fetches the names of the menu commands once. */
function useCommandNames() {
  const [names, setNames] = useState<main.MenuCommands | null>(null);
  useEffect(() => {
    let cancelled = false;
    api.Commands().then((n) => {
      if (!cancelled) setNames(n);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return names;
}

export type Command = "settings" | "download" | "downloads" | "history";

/**
 * useOutsideInput hands the interface what reaches Lasso from outside the
 * page: menu commands, and links from lasso://, the Dock, a .webloc or File ›
 * Paste Link.
 *
 * A link is collected rather than received, so one that arrived before the
 * page was listening — the lasso:// link that launched Lasso — is found on the
 * first look.
 */
export function useOutsideInput(handlers: { onCommand: (command: Command) => void; onLink: (url: string) => void }) {
  const names = useEventNames();
  const commands = useCommandNames();
  const latest = useLatest(handlers);

  useEffect(() => {
    if (!names || !commands) return;
    const collect = () =>
      api.TakeIncomingLink().then((link) => {
        if (link) latest.current.onLink(link);
      });
    const byName: Record<string, Command> = {
      [commands.settings]: "settings",
      [commands.download]: "download",
      [commands.downloads]: "downloads",
      [commands.history]: "history",
    };

    void collect();
    EventsOn(names.linkWaiting, () => void collect());
    EventsOn(names.menu, (name: string) => {
      const command = byName[name];
      if (command) latest.current.onCommand(command);
    });
    return () => {
      EventsOff(names.linkWaiting);
      EventsOff(names.menu);
    };
  }, [names, commands, latest]);
}
