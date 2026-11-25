import type { Metadata } from "next";
import "./style.css";

export const metadata: Metadata = {
  title: "NexusAI · Review workspace",
  description: "Fictional invoice review and cited policy answers.",
};
export default function Layout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
