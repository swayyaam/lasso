import React from "react";
import { createRoot } from "react-dom/client";
import { TooltipProvider } from "@lasso/ui";
import "./index.css";
import { App } from "./App";

const container = document.getElementById("root");
if (!container) throw new Error("no #root element");

createRoot(container).render(
  <React.StrictMode>
    {/* One provider for the whole app: Radix shares the open delay across a
        group, so moving between two icon buttons does not re-wait. */}
    <TooltipProvider>
      <App />
    </TooltipProvider>
  </React.StrictMode>,
);
