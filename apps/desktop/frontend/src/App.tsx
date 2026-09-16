import { useState } from "react";
import { api, core } from "./bindings";
import { useBinaryStatus, useQueue } from "./useBackend";
import { Thumbnail } from "./Thumbnail";

/**
 * Phase D smoke-test UI.
 *
 * This exists to prove the bindings and the event stream work end to end. The
 * designed interface replaces it in phase E; nothing here is styled beyond what
 * it takes to read.
 */
export function App() {
  const status = useBinaryStatus();
  const items = useQueue();

  const [url, setUrl] = useState("");
  const [pick, setPick] = useState<string>("best");
  const [metadata, setMetadata] = useState<core.Metadata | null>(null);
  const [command, setCommand] = useState("");
  const [error, setError] = useState("");

  // The loading state for metadata is not optional: resolving a link reaches
  // the network and takes a second or two even when everything is warm.
  const [resolving, setResolving] = useState(false);

  const options = (): core.Options =>
    ({ url, pick, subtitles: {}, enhancements: {}, playlist: {}, network: {}, output: {} }) as core.Options;

  async function resolve() {
    setError("");
    setMetadata(null);
    setResolving(true);
    try {
      setMetadata(await api.FetchMetadata(url));
    } catch (e) {
      setError(String(e));
    } finally {
      setResolving(false);
    }
  }

  async function download() {
    setError("");
    try {
      await api.Enqueue(options(), metadata?.title ?? "");
    } catch (e) {
      setError(String(e));
    }
  }

  async function showCommand() {
    setError("");
    try {
      setCommand(await api.ShowCommand(options()));
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <main style={{ fontFamily: "system-ui", padding: 24, color: "#f7f8f8", background: "#010102", minHeight: "100vh" }}>
      <h1 style={{ fontSize: 22, marginTop: 0 }}>Lasso — phase D</h1>

      <BinaryBanner status={status} />

      <section style={{ marginBottom: 24 }}>
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="Paste a link"
          style={{ width: 460, padding: 8, marginRight: 8 }}
        />
        <button onClick={resolve} disabled={!url || resolving}>
          {resolving ? "Resolving…" : "Fetch details"}
        </button>
      </section>

      {resolving && <p>Resolving link…</p>}

      {metadata && (
        <section style={{ marginBottom: 24, display: "flex", gap: 16 }}>
          <Thumbnail thumbnails={metadata.thumbnails} width={200} alt={metadata.title} />
          <div>
            <h2 style={{ fontSize: 18, margin: "0 0 4px" }}>{metadata.title}</h2>
            <p style={{ margin: "0 0 4px", opacity: 0.7 }}>
              {metadata.uploader} · {metadata.kind === "playlist" ? `${metadata.entries?.length ?? 0} items` : `${Math.round(metadata.duration)}s`}
            </p>
            <p style={{ margin: "0 0 8px", opacity: 0.7 }}>{metadata.formats?.length ?? 0} formats available</p>

            <select value={pick} onChange={(e) => setPick(e.target.value)}>
              {["best", "2160p", "1440p", "1080p", "720p", "audio-mp3", "audio-m4a", "audio-flac", "audio-opus"].map((p) => (
                <option key={p} value={p}>{p}</option>
              ))}
            </select>
            <button onClick={download} style={{ marginLeft: 8 }}>Download</button>
            <button onClick={showCommand} style={{ marginLeft: 8 }}>Show command</button>
          </div>
        </section>
      )}

      {command && (
        <pre style={{ background: "#0f1011", padding: 12, whiteSpace: "pre-wrap", fontSize: 12 }}>{command}</pre>
      )}

      {error && <p style={{ color: "#e5484d" }}>{error}</p>}

      <h2 style={{ fontSize: 18 }}>Queue ({items.length})</h2>
      {items.length === 0 && <p style={{ opacity: 0.6 }}>Nothing queued yet.</p>}
      {items.map((item) => (
        <QueueRow key={item.id} item={item} />
      ))}
    </main>
  );
}

function BinaryBanner({ status }: { status: Awaited<ReturnType<typeof api.BinaryStatus>> | null }) {
  if (!status) return <p style={{ opacity: 0.6 }}>Checking helper programs…</p>;
  if (status.ready) {
    return (
      <p style={{ opacity: 0.6, fontSize: 13 }}>
        Ready · {Object.entries(status.versions ?? {}).map(([name, v]) => `${name} ${String(v).split(" ")[0]}`).join(" · ")}
      </p>
    );
  }
  return (
    <div style={{ background: "#2a1416", border: "1px solid #e5484d", padding: 12, marginBottom: 16 }}>
      {status.problems?.map((p, i) => (
        <div key={i}>
          <strong>{p.message}</strong>
          <details><summary style={{ cursor: "pointer" }}>Details</summary><pre style={{ fontSize: 11 }}>{p.detail}</pre></details>
        </div>
      ))}
    </div>
  );
}

function QueueRow({ item }: { item: core.Item }) {
  const p = item.progress;
  const percent = p && p.percent >= 0 ? `${p.percent.toFixed(1)}%` : "—";
  const speed = p && p.speed > 0 ? `${(p.speed / 1_000_000).toFixed(1)} MB/s` : "";
  const eta = p && p.eta > 0 ? `${p.eta}s left` : "";

  return (
    <div style={{ borderBottom: "1px solid #23252a", padding: "12px 0" }}>
      <div style={{ display: "flex", justifyContent: "space-between" }}>
        <span>{item.title || item.options?.url}</span>
        <span style={{ opacity: 0.7 }}>{item.state}</span>
      </div>
      <div style={{ opacity: 0.7, fontSize: 13 }}>
        {percent} {speed} {eta} {p?.detail}
      </div>
      {item.message && (
        <div style={{ color: "#e5484d", fontSize: 13 }}>
          {item.message}
          {item.detail && (
            <details><summary style={{ cursor: "pointer" }}>Details</summary><pre style={{ fontSize: 11 }}>{item.detail}</pre></details>
          )}
        </div>
      )}
      <div style={{ marginTop: 6 }}>
        <button onClick={() => api.Cancel(item.id)} disabled={["done", "failed", "cancelled"].includes(item.state)}>
          Cancel
        </button>
        <button onClick={() => api.Retry(item.id)} disabled={!["failed", "cancelled"].includes(item.state)} style={{ marginLeft: 8 }}>
          Retry
        </button>
      </div>
    </div>
  );
}
