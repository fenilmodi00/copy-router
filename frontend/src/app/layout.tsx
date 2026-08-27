import { DataCacheProvider } from "@/components/DataCacheProvider";
import { TooltipProvider } from "@/components/molecules/Tooltip";
import type { Metadata } from "next";
import { Inter } from "next/font/google";

import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-inter",
});

export const metadata: Metadata = {
  title: "aiand-auto",
  description: "Router dashboard and settings",
  icons: {
    icon: "/ui/favicon-32x32.png",
    shortcut: "/ui/favicon-16x16.png",
    apple: "/ui/android-chrome-192x192.png",
  },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={inter.variable}>
      <body className="h-screen overflow-hidden font-sans antialiased">
        <DataCacheProvider>
          <TooltipProvider>{children}</TooltipProvider>
        </DataCacheProvider>
      </body>
    </html>
  );
}
