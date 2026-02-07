import { writable } from 'svelte/store';

interface User {
  id: number;
  username: string;
  role: 'admin' | 'operator' | 'readonly';
}

interface AuthState {
  token: string | null;
  user: User | null;
  isAuthenticated: boolean;
}

const getInitialState = (): AuthState => {
  if (typeof window !== 'undefined') {
    const token = localStorage.getItem('token');
    const userStr = localStorage.getItem('user');
    if (token && userStr) {
      try {
        const user = JSON.parse(userStr);
        return { token, user, isAuthenticated: true };
      } catch {
        // Invalid stored data
      }
    }
  }
  return { token: null, user: null, isAuthenticated: false };
};

function createAuthStore() {
  const { subscribe, set, update } = writable<AuthState>(getInitialState());

  return {
    subscribe,
    login: (token: string, user: User) => {
      if (typeof window !== 'undefined') {
        localStorage.setItem('token', token);
        localStorage.setItem('user', JSON.stringify(user));
      }
      set({ token, user, isAuthenticated: true });
    },
    logout: () => {
      if (typeof window !== 'undefined') {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
      }
      set({ token: null, user: null, isAuthenticated: false });
    },
    getToken: (): string | null => {
      if (typeof window !== 'undefined') {
        return localStorage.getItem('token');
      }
      return null;
    }
  };
}

export const auth = createAuthStore();
