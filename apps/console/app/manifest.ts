import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Veto",
    short_name: "Veto",
    description: "Review and control what your AI agents do.",
    start_url: "/",
    display: "standalone",
    background_color: "#f2f3f5",
    theme_color: "#17181c",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
    ],
  };
}
