# 🛰️ MiniSky

**High-Fidelity local emulator for Google Cloud Platform.**

**Official Website:** [minisky.bmics.com.ng](https://minisky.bmics.com.ng)

MiniSky provides a seamless, professional-grade development environment that emulates GCP services locally. It allows developers to test Infrastructure-as-Code (Terraform), Serverless functions, and complex data workflows without incurring cloud costs or requiring an internet connection.

[![Go Report Card](https://goreportcard.com/badge/github.com/qamarudeenm/minisky)](https://goreportcard.com/report/github.com/qamarudeenm/minisky)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Mini Movement](https://img.shields.io/badge/Mini-Family-blue.svg)](https://github.com/topics/mini-cloud)
[![High Fidelity](https://img.shields.io/badge/Fidelity-High-green.svg)](#)

---

## ✨ Features

- **🚀 29+ GCP Services**: Support for Compute Engine, GKE, Bigtable, Pub/Sub, Storage, Cloud SQL, Vertex AI, Artifact Registry, and more.
- **🖥️ Embedded Dashboard**: Real-time observability and resource management via a premium web UI.
- **🛠️ Terraform Ready**: First-class support for the official Google Cloud Terraform provider via custom endpoint routing.
- **🔌 Dynamic Registry**: Modular plugin system for community-led service contributions.
- **📦 Single Binary**: Developed entirely in Go. A single, ultra-lightweight binary where all services are lazy-loaded for maximum efficiency and sub-100ms startup times.

## 📋 Prerequisites
MiniSky requires the following tools installed and running on your local machine:
- **[Docker Desktop](https://www.docker.com/products/docker-desktop/)**: Used for high-fidelity service emulation (Compute, SQL, etc.).
- **[Git](https://git-scm.com/downloads)**: Required for installation and plugin management.

## 🚀 Quick Start

### Installation

**Linux & macOS:**
```bash
curl -sSL https://minisky.bmics.com.ng/install.sh | sh
```

**Windows — Direct Download (Recommended):**

Download the self-contained `minisky.exe` from the [latest GitHub release](https://github.com/qamarudeenm/minisky/releases/latest). No installer needed — just extract and run:

```powershell
# Download and extract
Invoke-WebRequest -Uri https://github.com/qamarudeenm/minisky/releases/latest/download/minisky_windows_amd64.zip -OutFile minisky.zip
Expand-Archive minisky.zip -DestinationPath C:\minisky

# Run
C:\minisky\minisky.exe start
```

> MiniSky stores all data in `%USERPROFILE%\.minisky\` — never in your working directory.

**Windows — Scoop (Alternative):**

```powershell
# Install Scoop if not already installed
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression

# Install MiniSky
scoop bucket add minisky https://github.com/qamarudeenm/scoop-bucket
scoop install minisky
```

### Start the Daemon
```bash
minisky start
```
- **API Gateway**: `http://localhost:8080`
- **Dashboard**: `http://localhost:8081`

### Uninstall
```bash
minisky uninstall
```
This removes all containers, networks, and data from `~/.minisky`. Then delete the binary to fully uninstall.

### Upgrading
To upgrade an existing installation to the latest version, you just need to replace the binary. Your data in `~/.minisky` is persistent and will be preserved automatically.

**Linux & macOS:**
Simply run the install script again:
```bash
curl -sSL https://minisky.bmics.com.ng/install.sh | sh
```

**Windows (Direct):**
1. Stop the running daemon (`minisky stop` or close the terminal).
2. Download the new `.zip` and overwrite your existing `minisky.exe`.

**Windows (Scoop):**
```powershell
scoop update minisky
```


## 🖥️ Platform Compatibility

MiniSky is cross-platform. All core GCP services work on every platform. BigQuery SQL execution uses an embedded [DuckDB](https://duckdb.org) engine which requires CGO — platforms where CGO is not available fall back to an in-memory mock that returns valid empty responses.

| Feature | Linux (amd64) | macOS (arm64) | Windows (Native) | Windows (WSL2 / Docker) |
| :--- | :---: | :---: | :---: | :---: |
| Compute / GKE / Storage | ✅ | ✅ | ✅ | ✅ |
| Pub/Sub / Cloud SQL / VPC | ✅ | ✅ | ✅ | ✅ |
| BigQuery SQL execution (DuckDB) | ✅ Full | ⚠️ Mock\* | ⚠️ Mock\* | ✅ Full |
| CGO build | Yes | No (v1.2.x) | No | Yes |

\* BigQuery queries return valid empty results. Schema inference, table creation, and insert operations work correctly. SQL execution is mocked pending CGO cross-compilation toolchain for darwin/arm64 and Windows.

> **Recommended alternative for macOS & Windows users who need full BigQuery SQL:**  
> Run MiniSky via **Docker Desktop** or **WSL2** on Windows — both use the Linux binary with full DuckDB support.

---

## 🗺️ Platform Roadmap — DuckDB / CGO

This roadmap tracks the work required to enable full DuckDB-powered BigQuery emulation on macOS and Windows native builds.

### macOS arm64 — DuckDB Status: `⚠️ Mocked (v1.2.x)`

The darwin/arm64 binary is currently built with `CGO_ENABLED=0`. DuckDB compiles ~700k lines of C++ at build time and requires a Darwin-targeting C++ cross-compiler to produce a macOS binary from our Linux release machine.

**Planned implementation (post v1.2.x):**

| Step | What | Status |
| :--- | :--- | :---: |
| 1 | Integrate `goreleaser-cross` Docker image into `release.sh --docker` | 🔜 Planned |
| 2 | Set darwin target: `CC=o64-clang`, `CGO_ENABLED=1` in `.goreleaser.yaml` | 🔜 Planned |
| 3 | Validate `minisky_darwin_arm64.tar.gz` DuckDB BQ execution on M-series Mac | 🔜 Planned |
| 4 | Update installer + compatibility table to ✅ | 🔜 Planned |

**Alternative paths under consideration:**
- `zig cc -target aarch64-macos` as a lightweight cross-compiler (5 min setup, no image pull)
- `osxcross` built from source (~30 min, most robust)

### Windows amd64 — DuckDB Status: `⚠️ Mocked (by design)`

Windows builds use `CGO_ENABLED=0` for maximum portability (no MSVC/mingw dependency chain for end users). DuckDB on Windows native requires either a MinGW64 toolchain or a pre-built DuckDB `.dll`.

**Planned implementation (post v1.2.x):**

| Step | What | Status |
| :--- | :--- | :---: |
| 1 | Evaluate shipping a pre-built `duckdb.dll` alongside the Windows binary | 🔜 Investigating |
| 2 | Or: ship a DuckDB-enabled Windows build via `goreleaser-cross` + MinGW64 | 🔜 Investigating |
| 3 | Or: document WSL2 as the canonical Windows BigQuery path | 🔜 Fallback |

> **Current recommended workaround:** Windows users needing full BigQuery SQL emulation should use **WSL2** + the Linux install script, or run `docker run` with the MiniSky Linux image.

---

## 📖 Documentation

- [CLI Reference](docs/cli_reference.md)
- [Terraform Guide](docs/terraform.md)
- [Changelog](CHANGELOG.md)
- [Contributor Guide](CONTRIBUTING.md)

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on how to build and register new service shims.

## 📄 License

MiniSky is released under the [MIT License](LICENSE).