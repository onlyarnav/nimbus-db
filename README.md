# NimbusDB — Distributed AI-Native Vector Database

NimbusDB is a cloud-native, distributed, AI-ready vector database platform built with Go, Rust, and Next.js, deployed on Kubernetes using Helm.

## Features & Architecture

- **AI-Native Vector Engine:** High-dimensional vector embeddings, HNSW ANN & exact similarity search with metadata filtering.
- **Microservices Architecture:** 10 decoupled microservices including Auth, Gateway, Control Plane, Metadata Service, Scheduler, Worker Nodes, Capacity Planner, SLA Monitor, Deployment Controller, and Dashboard.
- **Kubernetes Native:** Complete Helm charts with StatefulSets, Deployments, and automated Canary/Rolling update capabilities.

## Documentation

All project documentation, implementation phases, benchmarks, and simulation walkthroughs are available in the [`ai-docs/`](./ai-docs/) folder:

- [Project Status & Architecture Architecture](./ai-docs/PROJECT_STATUS.md)
- [Simulation Walkthrough & Verification](./ai-docs/SIMULATION_WALKTHROUGH.md)
- [Verification Checklist](./ai-docs/VERIFICATION_CHECKLIST.md)
- [Detailed Technical Documentation](./ai-docs/README.md)
