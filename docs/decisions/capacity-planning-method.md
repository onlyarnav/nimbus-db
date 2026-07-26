# Architectural Decision Record: Capacity Planning Forecasting Method

## Context
Phase 7 introduces the `capacity-planner` microservice (`services/capacity-planner`) to project future node infrastructure requirements (e.g., node capacity over a 7-day or 30-day forecast horizon) based on historical resource load metrics.

We must decide between a complex machine learning time-series forecasting model (e.g., ARIMA / Prophet) and a trend-based mathematical projection model.

## Decisions

### Trend-Based Linear Regression Projection
NimbusDB implements **ordinary least squares (OLS) linear regression** over historical time-series metric samples ($y = mx + b$).

### Rationale & Scope Boundaries
1. **Whiteboard-Defensible & Transparent**: Linear regression produces deterministic, mathematically verifiable projections ($m = \frac{n \sum (xy) - \sum x \sum y}{n \sum x^2 - (\sum x)^2}$) that can be easily explained in systems design interviews without obscuring logic behind opaque ML models.
2. **Minimal Operational Dependencies**: Avoids importing heavyweight Python/ML runtime sidecars into the Go control plane binary.
3. **Explicit Scope Boundary**: Documented as a deliberate architectural simplification. Machine learning based anomaly detection or non-linear forecasting is explicitly excluded to preserve project focus and engineering discipline.
