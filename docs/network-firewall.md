# MiniSky Network Isolation & Firewall Enforcement

MiniSky implements a robust **3-Level approach** to VPC networking and Firewall bindings. This bridges the gap between mocked state and actual underlying Data Plane (Docker) containers. 

## Level 1: L2 VPC Network Isolation

When a user creates a new VPC inside the MiniSky dashboard, it directly provisions a matching Docker isolated network bridge.

- VPC `my-vpc-1` → Docker network `minisky-vpc-my-vpc-1`
- VPC `my-vpc-2` → Docker network `minisky-vpc-my-vpc-2`

**Behaviour:**
- VMs assigned to `my-vpc-1` are physically isolated from VMs on `my-vpc-2`.
- You can no longer `ping` or `curl` across differing VPC networks.

## Level 2: Firewall Port Binding

Because Docker requires port bindings to be established at container creation time, MiniSky employs a **State Reconciliation Loop** for port matching.

When you create a firewall rule (e.g. `ALLOW tcp:80,443 INGRESS`), the system:
1. Identifying all running compute VMs that belong to the targeted VPC network.
2. Instructing the `ServiceManager` orchestrator to **tear down and re-provision** those specific compute containers.
3. Upon re-provision, dynamic Host port mappings (`-p 127.0.0.1::80`) are explicitly added via Docker's `PortBindings` API structure based on the active whitelist.
4. The ports natively map via the OS loopback IP, allowing you to access `127.0.0.1:<random-port>` directly.

Ports are visible within the `Compute Engine` drawer whenever a VM is assigned a port mapping!

## Level 3: Policy Context Evaluation

For connections coming directly into MiniSky's `:8080` Proxy API path (in future roadmap additions), we maintain an active in-memory `FirewallEntry` registry that checks:
- The destination VPC network.
- Protocol/Ports filtering.
- Source range checking (CIDR checking for `0.0.0.0/0`).

*Note: Level 3 is primarily informational for the Proxy as most local emulator traffic routes directly through the Level 2 OS port bindings. It stands as an architectural pillar for future HTTP Proxy load balancing.*
