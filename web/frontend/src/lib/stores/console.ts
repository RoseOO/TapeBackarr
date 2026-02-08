import { writable } from 'svelte/store';

export interface ConsoleEntry {
  id: string;
  level: 'info' | 'warn' | 'error' | 'success' | 'debug';
  message: string;
  timestamp: string;
  source?: string;
}

function createConsoleStore() {
  const { subscribe, update, set } = writable<ConsoleEntry[]>([]);

  // Load collapsed state from localStorage
  const getCollapsed = (): boolean => {
    if (typeof window === 'undefined') return true;
    return localStorage.getItem('console_collapsed') !== 'false';
  };

  const collapsed = writable<boolean>(getCollapsed());

  return {
    subscribe,
    collapsed,
    add: (entry: Omit<ConsoleEntry, 'id' | 'timestamp'>) => {
      const newEntry: ConsoleEntry = {
        ...entry,
        id: Date.now().toString() + Math.random().toString(36).substr(2, 5),
        timestamp: new Date().toISOString(),
      };
      update(entries => {
        const updated = [...entries, newEntry];
        // Keep last 500 entries
        return updated.slice(-500);
      });
    },
    clear: () => set([]),
    toggleCollapsed: () => {
      collapsed.update(v => {
        const newVal = !v;
        if (typeof window !== 'undefined') {
          localStorage.setItem('console_collapsed', String(newVal));
        }
        return newVal;
      });
    },
    setCollapsed: (val: boolean) => {
      collapsed.set(val);
      if (typeof window !== 'undefined') {
        localStorage.setItem('console_collapsed', String(val));
      }
    }
  };
}

export const consoleStore = createConsoleStore();
