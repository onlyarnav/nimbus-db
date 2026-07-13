# NimbusDB Node Health Dashboard

This is the Next.js control plane dashboard that provides real-time health monitoring of worker nodes in the NimbusDB cluster.

## Responsibilities

- Polls the Metadata Service REST endpoint (`GET /v1/nodes`) every 3 seconds.
- Displays node names, IDs, and cluster associations.
- Color-codes node states:
  - <span style="color:#10B981;font-weight:bold;">HEALTHY</span>: Node is active and reporting heartbeats.
  - <span style="color:#F59E0B;font-weight:bold;">UNHEALTHY</span>: No heartbeat received for 15s.
  - <span style="color:#EF4444;font-weight:bold;">DEAD</span>: No heartbeat received for 60s.
  - <span style="color:#EC4899;font-weight:bold;">OVERLOADED</span>: CPU/RAM/Disk stats are >90% for 3 consecutive check loops.
- Displays resource visualizers for CPU, Memory, and Disk usage percentage.
- Shows relative time elapsed since the last received heartbeat (e.g. "5s ago").

## Tech Stack

- **Framework**: Next.js 16.2 (App Router)
- **Language**: TypeScript
- **Styling**: Sleek modern dark mode (Slate-based theme) with embedded CSS styles.

## Getting Started

### Running the Dashboard

Ensure that the Metadata Service is running on `localhost:8080` (CORS is enabled by default to allow dashboard requests).

1. Install dependencies:
   ```bash
   npm install
   ```
2. Start the development server:
   ```bash
   npm run dev
   ```
3. Open http://localhost:3000 to view the live dashboard.
