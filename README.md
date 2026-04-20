# 🛰️ MiniSky

**High-Fidelity local emulator for Google Cloud Platform.**

MiniSky provides a seamless, professional-grade development environment that emulates GCP services locally. It allows developers to test Infrastructure-as-Code (Terraform), Serverless functions, and complex data workflows without incurring cloud costs or requiring an internet connection.

[![Go Report Card](https://goreportcard.com/badge/github.com/qamarudeenm/minisky)](https://goreportcard.com/report/github.com/qamarudeenm/minisky)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## ✨ Features

- **🚀 16+ GCP Services**: Support for Compute Engine, GKE, Bigtable, Pub/Sub, Storage, Cloud SQL, and more.
- **🖥️ Embedded Dashboard**: Real-time observability and resource management via a premium web UI.
- **🛠️ Terraform Ready**: First-class support for the official Google Cloud Terraform provider via custom endpoint routing.
- **🔌 Dynamic Registry**: Modular plugin system for community-led service contributions.
- **📦 Single Binary**: Distributed as a single, portable binary with zero external Go dependencies (uses Docker for backends).

## 🚀 Quick Start

### Installation
**Linux & macOS:**
```bash
curl -sSL https://raw.githubusercontent.com/qamarudeenm/minisky/main/install.sh | sh
```

**Windows (via Scoop):**
```powershell
scoop bucket add minisky https://github.com/qamarudeenm/scoop-bucket
scoop install minisky
```

### Start the Daemon
```bash
minisky start
```
- **API Gateway**: `http://localhost:8080`
- **Dashboard**: `http://localhost:8081`

## 📖 Documentation

- [Project Documentation](docs/architecture.md)

## 🖥️ Platform Compatibility

MiniSky is cross-platform, but some features have varying support levels:

| Feature | Linux | macOS | Windows (Native) | Windows (WSL2/Docker) |
| :--- | :---: | :---: | :---: | :---: |
| GKE / Compute / Storage | ✅ | ✅ | ✅ | ✅ |
| Pub/Sub / Cloud SQL | ✅ | ✅ | ✅ | ✅ |
| BigQuery SQL Execution | ✅ | ✅ | ⚠️ (Mock) | ✅ |
| **CGO Required** | Yes | Yes | No | Yes |

> **Note**: Native Windows builds use a `CGO_DISABLED=1` configuration for maximum portability. This means BigQuery SQL execution is mocked. For full high-fidelity BigQuery emulation on Windows, we recommend running MiniSky via **Docker Desktop** or **WSL2**.

## 📖 Documentation

- [CLI Reference](docs/cli_reference.md)
- [Terraform Guide](docs/terraform.md)
- [Contributor Guide](CONTRIBUTING.md)

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on how to build and register new service shims.

## 📄 License

MiniSky is released under the [MIT License](LICENSE).
