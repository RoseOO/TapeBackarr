import { writable } from 'svelte/store';

export interface Notification {
  id: string;
  type: 'info' | 'warning' | 'success' | 'error';
  category: string;
  title: string;
  message: string;
  details?: Record<string, any>;
  timestamp: string;
  read?: boolean;
}

function getDismissedIds(): Set<string> {
  if (typeof window === 'undefined') return new Set();
  try {
    const stored = localStorage.getItem('dismissed_notifications');
    return stored ? new Set(JSON.parse(stored)) : new Set();
  } catch {
    return new Set();
  }
}

function saveDismissedIds(ids: Set<string>) {
  if (typeof window === 'undefined') return;
  // Keep only last 500 dismissed IDs to prevent unbounded growth
  const arr = Array.from(ids).slice(-500);
  localStorage.setItem('dismissed_notifications', JSON.stringify(arr));
}

function createNotificationStore() {
  const { subscribe, update, set } = writable<Notification[]>([]);
  let eventSource: EventSource | null = null;
  let dismissedIds = getDismissedIds();

  return {
    subscribe,
    add: (notification: Notification) => {
      if (dismissedIds.has(notification.id)) return;
      update(n => {
        if (n.some(existing => existing.id === notification.id)) return n;
        const updated = [notification, ...n];
        return updated.slice(0, 200);
      });
    },
    markRead: (id: string) => {
      dismissedIds.add(id);
      saveDismissedIds(dismissedIds);
      update(n => n.map(item => item.id === id ? { ...item, read: true } : item));
    },
    markAllRead: () => {
      update(n => {
        for (const item of n) {
          dismissedIds.add(item.id);
        }
        saveDismissedIds(dismissedIds);
        return n.map(item => ({ ...item, read: true }));
      });
    },
    clear: () => {
      update(n => {
        for (const item of n) {
          dismissedIds.add(item.id);
        }
        saveDismissedIds(dismissedIds);
        return [];
      });
    },
    connect: () => {
      if (typeof window === 'undefined') return;
      const token = localStorage.getItem('token');
      if (!token) return;

      if (eventSource) {
        eventSource.close();
      }

      eventSource = new EventSource(`/api/v1/events/stream?token=${encodeURIComponent(token)}`);
      
      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as Notification;
          // Skip dismissed notifications
          if (dismissedIds.has(data.id)) return;
          update(n => {
            if (n.some(existing => existing.id === data.id)) return n;
            const updated = [data, ...n];
            return updated.slice(0, 200);
          });
        } catch (e) {
          // Ignore parse errors
        }
      };

      eventSource.onerror = () => {
        // Will auto-reconnect
      };
    },
    disconnect: () => {
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
    }
  };
}

export const notifications = createNotificationStore();
