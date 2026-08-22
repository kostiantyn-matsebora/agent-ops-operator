/**
 * THE BUILT-IN SET — `aops:<name>`.
 *
 * Icons ship INSIDE the surface that draws them. That is the whole point of the
 * scheme: an `aops:` reference needs no network, survives an air-gapped
 * install, and cannot break because a CDN moved. It is the only form guaranteed
 * on every surface, so it is what the chart uses.
 *
 * Paths are 24x24, single-colour, drawn with `currentColor` so they inherit the
 * text they sit beside and need no per-theme variant.
 *
 * Keep this SMALL and generic. It covers the domains the charts ship; anything
 * else is a `mdi:` name or a URL, which is exactly why those forms exist.
 */
export const BUILTIN_ICONS: Record<string, string> = {
  // Kubernetes — the helm/wheel, simplified to a single path.
  kubernetes:
    'M12 2 3.5 6v8L12 22l8.5-8V6L12 2Zm0 2.2 6.5 3.06v6.1L12 19.6l-6.5-6.24v-6.1L12 4.2Z'
    + 'M12 7a5 5 0 1 0 0 10 5 5 0 0 0 0-10Zm0 2a3 3 0 1 1 0 6 3 3 0 0 1 0-6Z',
  // A magnifier — observing, reading, changing nothing.
  observe:
    'M10 2a8 8 0 1 0 4.9 14.32l5.39 5.39 1.42-1.42-5.39-5.39A8 8 0 0 0 10 2Zm0 2a6 6 0 1 1 0 12 6 6 0 0 1 0-12Z',
  // A wrench — acting, changing things.
  operate:
    'M20.7 6.3a5 5 0 0 1-6.1 6.1L6.4 20.6a2 2 0 0 1-2.8-2.8l8.2-8.2a5 5 0 0 1 6.1-6.1l-3 3 .7 2.8 2.8.7 3-3Z',
  // A bell — something is firing.
  alert:
    'M12 2a6 6 0 0 0-6 6c0 3.6-1 5.3-1.8 6.2A1 1 0 0 0 5 16h14a1 1 0 0 0 .8-1.8C19 13.3 18 11.6 18 8a6 6 0 0 0-6-6Zm0 20a3 3 0 0 0 3-3H9a3 3 0 0 0 3 3Z',
  // A house — the home lane.
  home: 'M12 3 2 12h3v9h6v-6h2v6h6v-9h3L12 3Z',
  // A speech bubble — a chat surface.
  chat:
    'M4 4h16a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H9l-5 4V6a2 2 0 0 1 2-2Z',
  // A person — somebody typed this.
  user:
    'M12 12a5 5 0 1 0 0-10 5 5 0 0 0 0 10Zm0 2c-4.4 0-8 2.2-8 5v3h16v-3c0-2.8-3.6-5-8-5Z',
  // A robot head — the agent answered.
  agent:
    'M12 2a1 1 0 0 1 1 1v2h3a4 4 0 0 1 4 4v7a4 4 0 0 1-4 4H8a4 4 0 0 1-4-4V9a4 4 0 0 1 4-4h3V3a1 1 0 0 1 1-1Z'
    + 'M9 11a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Zm6 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Z',
  // A gear — the system speaking on its own behalf.
  system:
    'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8Zm0 2a2 2 0 1 1 0 4 2 2 0 0 1 0-4Z'
    + 'M10.9 2h2.2l.4 2.2 1.7.7 1.8-1.3 1.5 1.5-1.3 1.8.7 1.7 2.2.4v2.2l-2.2.4-.7 1.7 1.3 1.8-1.5 1.5-1.8-1.3-1.7.7-.4 2.2h-2.2l-.4-2.2-1.7-.7-1.8 1.3-1.5-1.5 1.3-1.8-.7-1.7L2 13.1v-2.2l2.2-.4.7-1.7-1.3-1.8 1.5-1.5 1.8 1.3 1.7-.7L10.9 2Z',
  // A cube — a generic workload.
  workload:
    'M12 2 3 7v10l9 5 9-5V7l-9-5Zm0 2.3 6.5 3.6L12 11.5 5.5 7.9 12 4.3ZM5 9.6l6 3.3v6.5l-6-3.3V9.6Zm8 9.8v-6.5l6-3.3v6.5l-6 3.3Z',
}
