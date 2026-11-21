import { defineStore } from 'pinia';
import { shallowRef } from 'vue';

export interface MenuItem {
  label: string;
  hint?: string;
  /** Marks the option that is already in effect. */
  current?: boolean;
  onPick?: () => void;
}

export interface MenuRequest {
  title?: string;
  search?: boolean;
  placeholder?: string;
  items: MenuItem[];
}

export interface OpenMenu extends MenuRequest {
  x: number;
  y: number;
}

export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  message: string;
  actions: ToastAction[];
}

export interface Confirmation {
  message: string;
  confirmLabel: string;
  onConfirm: () => void;
}

// The menu's own size plus a margin, so it never opens past the window edge.
const MENU_FOOTPRINT = { width: 272, height: 340 };
const EDGE_MARGIN = 8;
const GAP_BELOW_ANCHOR = 4;
const TOAST_LIFETIME_MS = 8000;

/** The floating layer: at most one menu, one toast and one confirmation at a time. */
export const useOverlayStore = defineStore('overlay', () => {
  const menu = shallowRef<OpenMenu | null>(null);
  const toast = shallowRef<Toast | null>(null);
  const confirmation = shallowRef<Confirmation | null>(null);

  let toastTimer: ReturnType<typeof setTimeout> | undefined;

  function openMenu(anchor: Element, request: MenuRequest): void {
    const rect = anchor.getBoundingClientRect();
    const x = Math.max(EDGE_MARGIN, Math.min(rect.left, window.innerWidth - MENU_FOOTPRINT.width));
    const y = Math.max(EDGE_MARGIN, Math.min(rect.bottom + GAP_BELOW_ANCHOR, window.innerHeight - MENU_FOOTPRINT.height));
    menu.value = { ...request, x, y };
  }

  function closeMenu(): void {
    menu.value = null;
  }

  function dismissToast(): void {
    clearTimeout(toastTimer);
    toast.value = null;
  }

  function showToast(message: string, actions: ToastAction[] = []): void {
    clearTimeout(toastTimer);
    toast.value = { message, actions };
    toastTimer = setTimeout(dismissToast, TOAST_LIFETIME_MS);
  }

  function confirm(request: Confirmation): void {
    confirmation.value = request;
  }

  function closeConfirmation(): void {
    confirmation.value = null;
  }

  function closeAll(): void {
    closeMenu();
    closeConfirmation();
    dismissToast();
  }

  return { menu, toast, confirmation, openMenu, closeMenu, showToast, dismissToast, confirm, closeConfirmation, closeAll };
});
