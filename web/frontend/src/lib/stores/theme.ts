import { writable } from 'svelte/store';

type Theme = 'dark' | 'light';

function createThemeStore() {
  const stored = typeof window !== 'undefined' ? localStorage.getItem('theme') as Theme : null;
  const initial: Theme = stored || 'dark';
  
  const { subscribe, set, update } = writable<Theme>(initial);

  return {
    subscribe,
    toggle: () => {
      update(current => {
        const next = current === 'dark' ? 'light' : 'dark';
        if (typeof window !== 'undefined') {
          localStorage.setItem('theme', next);
          document.documentElement.setAttribute('data-theme', next);
        }
        return next;
      });
    },
    init: () => {
      if (typeof window !== 'undefined') {
        const stored = localStorage.getItem('theme') as Theme || 'dark';
        document.documentElement.setAttribute('data-theme', stored);
        set(stored);
      }
    }
  };
}

export const theme = createThemeStore();
