import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "sauron",
  description: "Passive observation dashboard",
};

const themeInitScript = `(function(){try{var s=localStorage.getItem('theme');var m=window.matchMedia('(prefers-color-scheme: dark)').matches;document.documentElement.setAttribute('data-theme', s || (m?'dark':'light'));}catch(e){document.documentElement.setAttribute('data-theme','light');}})();`;

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
