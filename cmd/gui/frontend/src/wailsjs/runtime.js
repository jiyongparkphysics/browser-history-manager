// Wails v2 runtime bridge — re-exports from the Wails-injected runtime.
// During development, Wails injects these globals; during production builds
// they are available on the window object.

export function EventsOn(eventName, callback) {
  return window.runtime.EventsOn(eventName, callback);
}

export function EventsOff(eventName) {
  return window.runtime.EventsOff(eventName);
}

export function EventsEmit(eventName, ...data) {
  return window.runtime.EventsEmit(eventName, ...data);
}

export function LogDebug(message) {
  return window.runtime.LogDebug(message);
}

export function LogInfo(message) {
  return window.runtime.LogInfo(message);
}

export function LogError(message) {
  return window.runtime.LogError(message);
}
