// SPDX-License-Identifier: AGPL-3.0-only
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

export type ThemeChoice = "light" | "dark" | "system";

interface ThemeContextValue {
  choice: ThemeChoice;
  effective: "light" | "dark";
  setChoice: (next: ThemeChoice) => void;
  toggle: () => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function systemTheme(): "light" | "dark" {
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function storedChoice(): ThemeChoice {
  try {
    const value = localStorage.getItem("chastedesk-theme");
    if (value === "light" || value === "dark" || value === "system") return value;
  } catch {
    // Local storage can be unavailable in privacy modes.
  }
  return "system";
}

function applyChoice(choice: ThemeChoice): "light" | "dark" {
  const effective = choice === "system" ? systemTheme() : choice;
  document.documentElement.setAttribute("data-theme", effective);
  document.documentElement.style.colorScheme = effective;
  return effective;
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [choice, setChoiceState] = useState<ThemeChoice>(() => storedChoice());
  const [effective, setEffective] = useState<"light" | "dark">(() =>
    document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light",
  );

  useEffect(() => {
    setEffective(applyChoice(choice));
  }, [choice]);

  useEffect(() => {
    if (choice !== "system") return;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = () => setEffective(applyChoice("system"));
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, [choice]);

  const setChoice = useCallback((next: ThemeChoice) => {
    setChoiceState(next);
    try {
      if (next === "system") localStorage.removeItem("chastedesk-theme");
      else localStorage.setItem("chastedesk-theme", next);
    } catch {
      // The visual choice still applies for this tab.
    }
  }, []);

  const toggle = useCallback(() => {
    setChoice(effective === "dark" ? "light" : "dark");
  }, [effective, setChoice]);

  const value = useMemo(() => ({ choice, effective, setChoice, toggle }), [choice, effective, setChoice, toggle]);
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext);
  if (!context) throw new Error("useTheme must be used inside ThemeProvider");
  return context;
}
