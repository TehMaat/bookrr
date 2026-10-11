import { createContext, useContext, useEffect, useState, type ReactNode } from "react"

import { findAccent, logoUrl, type Accent, type AccentId } from "@/lib/accent"

type Theme = "light" | "dark" | "system"

const ThemeContext = createContext<{
  theme: Theme
  setTheme: (t: Theme) => void
  accent: Accent
  setAccent: (a: AccentId) => void
}>({
  theme: "system",
  setTheme: () => {},
  accent: findAccent(null),
  setAccent: () => {},
})

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null /* storage unavailable */
  }
}

function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* storage unavailable */
  }
}

function stored(): Theme {
  const t = read("bookrr-theme")
  return t === "light" || t === "dark" || t === "system" ? t : "system"
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(stored)
  const [accent, setAccentState] = useState<Accent>(() => findAccent(read("bookrr-accent")))

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)")
    const apply = () => {
      const dark = theme === "dark" || (theme === "system" && media.matches)
      document.documentElement.classList.toggle("dark", dark)
    }
    apply()
    media.addEventListener("change", apply)
    return () => media.removeEventListener("change", apply)
  }, [theme])

  useEffect(() => {
    const root = document.documentElement.style
    root.setProperty("--brand-light", accent.light.brand)
    root.setProperty("--brand-light-foreground", accent.light.fg)
    root.setProperty("--brand-dark", accent.dark.brand)
    root.setProperty("--brand-dark-foreground", accent.dark.fg)
    const icon = document.querySelector<HTMLLinkElement>("link[rel='icon']")
    if (icon) icon.href = logoUrl(accent)
  }, [accent])

  const setTheme = (t: Theme) => {
    write("bookrr-theme", t)
    setThemeState(t)
  }

  const setAccent = (id: AccentId) => {
    write("bookrr-accent", id)
    setAccentState(findAccent(id))
  }

  return <ThemeContext.Provider value={{ theme, setTheme, accent, setAccent }}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  return useContext(ThemeContext)
}
