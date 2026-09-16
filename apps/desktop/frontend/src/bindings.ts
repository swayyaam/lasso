// A single place to import the Wails-generated bindings from, so components
// never reach into the generated tree directly and a regenerate cannot ripple
// through the app.
export * as api from "../wailsjs/go/main/App";
export { core, main, binaries, presets } from "../wailsjs/go/models";
export { EventsOn, EventsOff } from "../wailsjs/runtime/runtime";
