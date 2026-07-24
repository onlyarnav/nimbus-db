# Architectural Decision Record: Observability Alerting Infrastructure

## Context
In Phase 5, NimbusDB requires real-time alerting for SLA-critical conditions (sustained latency > 500ms p95, node/region failure, replication lag spikes, and high error rates). We must select an alerting system.

## Decision
NimbusDB will use **Prometheus Alertmanager** combined with a custom lightweight **Webhook Receiver** service.

## Rationale
1. **Separation of Concerns**: Prometheus handles rule evaluation (`alert.rules.yml`), while Alertmanager handles grouping, deduplication, and routing to webhook targets.
2. **Deterministic Verification**: Sending alerts to a local HTTP webhook receiver allows end-to-end automated integration testing without depending on external third-party services (Slack/PagerDuty).
3. **Resume Authenticity**: Demonstrates hands-on experience configuring production Prometheus alert rules (`expr`, `for`, `labels`, `annotations`).

## Implications
- Alert rules are defined in `deploy/prometheus/alert.rules.yml`.
- A Go webhook receiver daemon listens on port `9099` to log and record fired alerts for integration tests.
