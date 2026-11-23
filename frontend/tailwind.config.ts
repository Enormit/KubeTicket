import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
        display: ["var(--font-display)", "var(--font-sans)", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "monospace"],
      },
      colors: {
        kt: {
          ground: "var(--kt-ground)",
          surface: "var(--kt-surface)",
          alt: "var(--kt-surface-alt)",
          ink: "var(--kt-ink)",
          muted: "var(--kt-ink-muted)",
          accent: "var(--kt-accent)",
          emerald: "var(--kt-emerald)",
          amber: "var(--kt-amber)",
          danger: "var(--kt-danger)",
          rule: "var(--kt-rule)",
        },
      },
      transitionTimingFunction: {
        "expo-out": "cubic-bezier(0.16, 1, 0.3, 1)",
      },
    },
  },
  plugins: [],
};

export default config;
