# MiniSky Adoption Playbook: Retention & Advocacy

Promotion gets people to the door; adoption keeps them in the room. This playbook focuses on the Developer Experience (DX) and turning users into advocates.

## 1. Lowering the Barrier to Entry
- **Zero-Config Defaults:** MiniSky should run with zero environment variables initially.
- **Auto-Provisioning:** If a user calls a service that isn't running, MiniSky should "lazy-load" it on the fly with a clear UI notification in the terminal.
- **Port 8080 Standard:** Stick to the standard GCP API gateway model. Don't make users memorize 15 different port numbers.

## 2. The "Trust" Factor
To win over enterprise users, we must prove fidelity:
- **Test Suite Transparency:** Publish the results of our parity tests (running the official Google SDK tests against MiniSky).
- **Security First:** Explicitly document that MiniSky is 100% local and sends no data to external servers.

## 3. Advocacy Loop (The Flywheel)
1.  **Surprise & Delight:** When a user catches a bug locally that would have failed in CI, show a small terminal message: *"MiniSky just saved you a 10-minute CI run. Consider sharing this on X/LinkedIn!"*
2.  **Contribution Tiers:**
    *   *Supporter:* Star the repo.
    *   *Contributor:* Fix a minor Discovery Doc validation error.
    *   *Maintainer:* Own a service shim (e.g., "The BigQuery Lead").
3.  **The "Cloud Cost Calculator":** Add a feature to the dashboard that estimates how much GCP money MiniSky has saved the user based on their API call volume.

## 4. Onboarding Checklist for Users
- [ ] Install MiniSky binary.
- [ ] Run `minisky start`.
- [ ] Connect the Google Terraform Provider.
- [ ] Create your first "Local-Only" GCS Bucket.
- [ ] Join the Discord for real-time support.

## 5. Handling Feedback
- **"Feature Request" Bot:** Automate the process of converting "Missing Service" comments into GitHub Issues with a "Service Parity" tag.
- **Rapid Iteration:** Aim for weekly "Service Update" releases to show momentum.
