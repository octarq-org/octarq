import { create } from "zustand";
import { NotificationFilter } from "./types";

interface NotificationUIState {
  isDrawerOpen: boolean;
  filter: NotificationFilter;
  selectedId: number | null;
  openDrawer: () => void;
  closeDrawer: () => void;
  toggleDrawer: () => void;
  setFilter: (filter: NotificationFilter) => void;
  setSelectedId: (id: number | null) => void;
}

export const useNotificationUIStore = create<NotificationUIState>((set) => ({
  isDrawerOpen: false,
  filter: "all",
  selectedId: null,
  openDrawer: () => set({ isDrawerOpen: true }),
  closeDrawer: () => set({ isDrawerOpen: false }),
  toggleDrawer: () => set((state) => ({ isDrawerOpen: !state.isDrawerOpen })),
  setFilter: (filter) => set({ filter }),
  setSelectedId: (selectedId) => set({ selectedId }),
}));
