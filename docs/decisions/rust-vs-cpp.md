# Architectural Decision Record: Storage Engine Programming Language

## Context

The storage engine of NimbusDB (built in Phase 3) requires manual memory management, predictable latency, raw control over byte layouts (for pages, WAL, and B+Trees), and high concurrent performance. We had to choose between Rust and C++ as the implementation language for the storage engine / Node Agent.

## Decision

We will implement the Storage Engine and Node Agent in **Rust**.

## Rationale

1. **Memory Safety without Garbage Collection**: Rust guarantees spatial and temporal memory safety at compile-time (no use-after-free, double-free, or data races) without the overhead of a garbage collector. This is critical for building a highly concurrent, robust database storage engine.
2. **Modern Tooling & Dependency Management**: Cargo provides excellent package management, testing frameworks, and dependency control out-of-the-box, accelerating development speed compared to C++ CMake/package-manager setups.
3. **Safe Concurrency**: Rust's `Send` and `Sync` traits ensure compile-time thread safety, reducing the risk of hard-to-debug concurrency bugs in our page layout caching and WAL write path.
4. **Microsoft Alignment**: Microsoft is actively adopting Rust for systems programming and rewriting core components of Windows and Azure infrastructure in Rust due to its safety and performance profile.

## Implications

- The Node Agent directory (`services/node-agent`) will be initialized as a Cargo Rust project in Phase 3.
- C++ will not be used in the storage engine.
- Rust tooling (rustup, cargo, clippy) will be required in the local development environment and GitHub Actions CI pipelines.
