import { WindowSetBackgroundColour } from "./bindings";

/**
 * Light or dark, as data-appearance on <html>, which is what tokens.css reads.
 *
 * The Appearance setting decides. Under Auto — an empty setting — the system
 * does, through prefers-color-scheme, and the page follows it live. The native
 * window is given the same setting by the backend, so the traffic lights and
 * any menu agree with the page.
 */
export type Appearance = "light" | "dark";

const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
let chosen = "";

export function resolveAppearance(choice: string, systemIsDark: boolean): Appearance {
  if (choice === "light" || choice === "dark") return choice;
  return systemIsDark ? "dark" : "light";
}

/**
 * startAppearance runs before the first render. Settings arrive a moment
 * later; until then the system's answer stands, and the window was already
 * created in the saved appearance, so the two rarely disagree.
 */
export function startAppearance() {
  systemDark.addEventListener("change", apply);
  apply();
}

/** setAppearance applies the saved setting. */
export function setAppearance(choice: string) {
  chosen = choice;
  apply();
}

function apply() {
  const next = resolveAppearance(chosen, systemDark.matches);
  const root = document.documentElement;
  if (root.dataset.appearance === next) return;
  // A switch, not the first paint: hold every transition for the frame, or
  // whatever animates its colour fades across while the rest has flipped.
  const switching = root.dataset.appearance !== undefined;
  if (switching) root.classList.add("appearance-switching");
  root.dataset.appearance = next;
  matchWindow();
  if (switching) {
    requestAnimationFrame(() => requestAnimationFrame(() => root.classList.remove("appearance-switching")));
  }
}

/**
 * The window has a colour of its own, which shows wherever a resize outruns
 * the page. It is kept the canvas's, read from the token rather than repeated
 * here.
 */
function matchWindow() {
  const canvas = getComputedStyle(document.documentElement).getPropertyValue("--color-canvas").trim();
  const hex = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(canvas);
  if (!hex) return;
  const [r, g, b] = hex.slice(1).map((pair) => parseInt(pair, 16));
  try {
    WindowSetBackgroundColour(r, g, b, 255);
  } catch {
    // Outside Wails — a plain browser during development — there is no window.
  }
}
