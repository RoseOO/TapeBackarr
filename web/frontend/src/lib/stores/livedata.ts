import { writable, derived } from 'svelte/store';
import { notifications } from './notifications';

/**
 * Reactive data store that auto-refreshes when SSE events arrive.
 * Pages subscribe to specific data categories and get notified when data changes.
 */

// Track which categories have been invalidated by SSE events
const invalidations = writable<Record<string, number>>({});

// Category mapping from SSE event categories to data types
const categoryMap: Record<string, string[]> = {
  'tape': ['tapes', 'drives', 'dashboard'],
  'drive': ['drives', 'dashboard'],
  'backup': ['backup-sets', 'jobs', 'dashboard'],
  'job': ['jobs', 'dashboard'],
  'pool': ['pools', 'dashboard'],
  'restore': ['backup-sets', 'dashboard'],
  'system': ['dashboard'],
  'proxmox': ['proxmox'],
};

/**
 * Get a derived store that changes whenever data of a given category is invalidated.
 * Pages can use this as a reactive dependency to trigger refetches.
 */
export function dataVersion(category: string) {
  return derived(invalidations, ($inv) => $inv[category] || 0);
}

/**
 * Manually invalidate a category (e.g. after a local mutation)
 */
export function invalidateData(...categories: string[]) {
  invalidations.update(inv => {
    const updated = { ...inv };
    for (const cat of categories) {
      updated[cat] = (updated[cat] || 0) + 1;
    }
    return updated;
  });
}

// Hook into the existing notification store's SSE connection.
// When notifications arrive, we invalidate relevant data categories.
let lastProcessedId = '';

export function processSSEEvent(event: { category?: string; type?: string; id?: string }) {
  if (!event.id || event.id === lastProcessedId) return;
  lastProcessedId = event.id;

  const sseCategory = event.category || '';
  const targets = categoryMap[sseCategory] || ['dashboard'];

  invalidateData(...targets);
}
