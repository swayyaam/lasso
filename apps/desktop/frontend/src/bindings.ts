// A single place to import the Wails-generated bindings from, so components
// never reach into the generated tree directly and regenerating cannot ripple
// through the app.
export * as api from "../wailsjs/go/main/App";
export { binaries, core, history, main, presets } from "../wailsjs/go/models";
export { EventsOff, EventsOn } from "../wailsjs/runtime/runtime";
