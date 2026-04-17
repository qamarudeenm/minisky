# MiniSky React UI Component Architecture

To build the high-fidelity dashboard presented in the mockups, the MiniSky frontend will be structured as a modern Single Page Application (SPA). The recommended stack is **Vite + React (TypeScript) + Material UI (MUI)** to mirror the Google Cloud Console experience.

## 1. Directory Structure

```text
src/
├── components/
│   ├── layout/
│   │   ├── TopHeader.tsx        # Logo, Search bar, Project Selector
│   │   └── NavigationRail.tsx   # Left sidebar for Dashboard, LRO, IAM
│   ├── dashboard/
│   │   ├── ServiceGrid.tsx      # Masonry/Grid layout for services
│   │   ├── ServiceCard.tsx      # Individual service control (Toggle, Metrics)
│   │   └── SparklineChart.tsx   # Lightweight SVG charts for CPU/RAM
│   ├── operations/
│   │   ├── OperationsView.tsx   # Dark-mode LRO visualizer
│   │   ├── LROTable.tsx         # Data grid for active/completed operations
│   │   └── StatusBadge.tsx      # Spinners & Progress Bars
│   └── iam/
│       └── IamPolicyTable.tsx   # View local RBAC rules
├── hooks/
│   ├── useServices.ts       # Fetches from /api/v1/services
│   ├── useOperations.ts     # Polls /api/v1/operations
│   └── useLogsStream.ts     # Manages WebSocket to /api/v1/logs/stream
└── App.tsx                  # Main Router
```

---

## 2. Component Design & Interactivity

### A. The `ServiceCard` Component
This is the core interactive element on the main dashboard.
- **State mappings:** 
  - `RUNNING` -> Blue/Green aesthetics, Sparklines active, Toggle `ON`.
  - `SLEEPING` -> Gray aesthetics, metrics frozen, Toggle `OFF`.
- **Interaction:** Clicking the power toggle invokes `POST /api/v1/services/{id}/toggle`. The card immediately enters a `PROVISIONING` loading state until the Daemon responds.

### B. High-Fidelity Feedback Loops (LRO Engine)
The Operations Tracking view heavily relies on polling or WebSockets.
- The `useOperations.ts` hook polls the Daemon every 2 seconds.
- Within `LROTable.tsx`, specific states map to visual feedback:
  - `PROVISIONING`: Indeterminate Circular Progress (Spinner).
  - `STAGING`: Determinate Linear Progress bar mapping to `progress_percent`.
  - `DONE`: Green checkmark icon.

### C. System State Validation
- **Project Selector Overlay:** Found in `TopHeader.tsx`. Changing the active project ID updates a global React Context, which attaches `x-minisky-project: {project_id}` headers to all internal UI API calls.
- **IAM visualizer (`IamPolicyTable.tsx`):** Reads from `/api/v1/iam/policies`. If a developer assigns a mock restriction locally, it is immediately reflected in this table to show what the active "dummy credentials" can access.

---

## 3. Theming & Styling

To match GCP perfectly without building everything from scratch:
- **Component Library:** Utilize `@mui/material` (Material UI). GCP Console follows Google's Material Design principles.
- **Font Stack:** Apply `Roboto` or `Google Sans` globally.
- **Palette Mode:** The main `ThemeProvider` will toggle between light mode (Dashboard view) and a custom dark steel gray theme (for Operations/Terminal logs view).
- **Icons:** Standard `Material Icons` accurately represent services and actions.
