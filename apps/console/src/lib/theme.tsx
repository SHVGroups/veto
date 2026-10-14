"use client";

import { createContext, useCallback, useContext, useEffect, type ReactNode } from "react";
import { useStored } from "./stored";

export type ThemeChoice = "system" | "light" | "dark";

const STORAGE_KEY = "veto.theme";

/**
 * Runs before the first paint (inlined in the document head) so a saved
 * theme never flashes the other one.
 */
export const themeBootScript = `try{var t=localStorage.getItem("${STORAGE_KEY}");if(t==="light"||t==="dark")document.documentElement.dataset.theme=t}catch(e){}`;

const ThemeContext = createContext<{ theme: ThemeChoice; setTheme: (t: ThemeChoice) => void }>({
  theme: "system",
  setTheme: () => {},
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, store] = useStored<ThemeChoice>(STORAGE_KEY, "system", ["light", "dark"]);

  // Keep the document attribute in step with the stored choice, including
  // changes made in another tab.
  useEffect(() => {
    const root = document.documentElement;
    if (theme === "system") delete root.dataset.theme;
    else root.dataset.theme = theme;
  }, [theme]);

  const setTheme = useCallback((t: ThemeChoice) => store(t === "system" ? null : t), [store]);

  return <ThemeContext.Provider value={{ theme, setTheme }}>{children}</ThemeContext.Provider>;
}

export const useTheme = () => useContext(ThemeContext);
