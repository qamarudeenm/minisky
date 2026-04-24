# MiniSky Product Launch Checklist

This checklist ensures every "Launch Event" (Product Hunt, Show HN, v1.0 Release) is executed with maximum impact.

## 1. Technical Readiness
- [ ] **Fidelity Smoke Test:** Run the "Standard Demo" (Terraform + GCS + Pub/Sub) to ensure no regressions.
- [ ] **Installation Validation:** Verify `curl | bash` works on Linux, macOS, and WSL.
- [ ] **Dashboard Polish:** Ensure all charts and logs render correctly without JS errors.
- [ ] **Version Tagging:** Properly tag the release on GitHub with a detailed changelog.

## 2. Marketing Assets
- [ ] **The "Hero" Video:** 60-90 second screen recording showing:
    - `minisky start`
    - Visual Dashboard in action.
    - Terraform apply catching an LRO transition.
- [ ] **Screenshot Gallery:** High-res screenshots of the Dashboard, BigQuery Workspace, and Networking View.
- [ ] **Social Media Kit:** Ready-to-use posts for the core team and early advocates.

## 3. Distribution Channels
- [ ] **Hacker News:** Submit as "Show HN: MiniSky - A High-Fidelity LocalStack for GCP".
- [ ] **Product Hunt:** Schedule for a Tuesday/Wednesday launch. Prepare the "First Comment" explaining the mission.
- [ ] **Reddit:** Cross-post to `r/googlecloud`, `r/devops`, `r/terraform`, and `r/opensource`.
- [ ] **LinkedIn:** All team members to share the "GCP on Localhost" narrative.

## 4. Community Engagement
- [ ] **Discord/Slack Ready:** Ensure the "Support" channel is monitored for the first 48 hours post-launch.
- [ ] **Issue Triage:** Have a team member dedicated to labeling and acknowledging new GitHub issues immediately.
- [ ] **The "Thank You" Bot:** Automated responses for new stargazers or contributors.

## 5. Post-Launch Analysis
- [ ] **Traffic Sources:** Identify which channel (HN, Reddit, etc.) drove the most quality engagement.
- [ ] **Star Velocity:** Track stars/day to measure momentum.
- [ ] **Feedback Log:** Consolidate all "I wish it did X" comments into the [Promotion Roadmap](promotion_roadmap.md).
