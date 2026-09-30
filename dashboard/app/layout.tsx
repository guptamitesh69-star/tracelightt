import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "TraceLight — LLM observability",
  description: "Real-time trace monitoring for production AI systems",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>{children}</body></html>;
}