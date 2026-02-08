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

function createNotificationStore() {
  const { subscribe, update, set } = writable<Notification[]>([]);
  let eventSource: EventSource | null = null;

  return {
    subscribe,
    add: (notification: Notification) => {
      update(n => {
        const updated = [notification, ...n];
        // Keep max 200 notifications
        return updated.slice(0, 200);
      });
    },
    markRead: (id: string) => {
      update(n => n.map(item => item.id === id ? { ...item, read: true } : item));
    },
    markAllRead: () => {
      update(n => n.map(item => ({ ...item, read: true })));
    },
    clear: () => set([]),
    connect: () => {
      if (typeof window === 'undefined') return;
      const token = localStorage.getItem('token');
      if (!token) return;

      // Close existing connection
      if (eventSource) {
        eventSource.close();
      }

      eventSource = new EventSource(`/api/v1/events/stream?token=${encodeURIComponent(token)}`);
      
      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as Notification;
          update(n => {
            // Avoid duplicates by ID
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
