# MiniSky Community Response Templates

This document contains standardized templates for interacting with the MiniSky community across GitHub, Medium, LinkedIn, and Twitter. Our voice is **professional, developer-centric, and passionate about open-source fidelity.**

---

## 1. General Praise / New User Welcome
**Use for:** Comments like "Great job," "Finally a GCP emulator," or users coming from MiniStack/MiniBlue.

> "Thank you so much for the support! We’re thrilled to see the 'Mini' movement expanding to GCP. Our goal is to make local development feel exactly like the real cloud, so you can stop paying for the privilege of running code on your own machine.
>
> If you're just getting started, check out our [User Guide](https://github.com/qamarudeenm/minisky/blob/main/docs/user-guide.md) and let us know if there’s anything we can do to improve your workflow!"

---

## 2. The "MiniStack / MiniBlue" Pivot
**Use for:** Users mentioning they use other local cloud emulators and are looking for a GCP equivalent.

> "Welcome to the family! We’re big fans of what MiniStack and MiniBlue have done for the AWS and Azure ecosystems. We built MiniSky to fill that same gap for GCP—focusing on high-fidelity features like Long-Running Operations (LROs) and Discovery Document validation that standard mocks usually skip.
>
> We’d love to hear how your experience comparing the platforms goes. If there’s a specific feature from the other 'Mini' tools you’d like to see here, we're all ears!"

---

## 3. Terraform / Infrastructure Questions
**Use for:** Users asking about Terraform integration or "it works on my machine" bugs.

> "That’s exactly why we built MiniSky! Most emulators fail because they don't handle asynchronous state changes. MiniSky implements a full LRO (Long-Running Operation) engine, so your Terraform provider can poll for resource readiness just like it does in production.
>
> If you hit any specific provider errors, please share your `terraform.tfvars` (with redacted secrets) in a GitHub issue so we can validate the API contract against the official Discovery Docs."

---

## 4. Feature Requests (The "Not Supported Yet" Response)
**Use for:** Users asking for Cloud KMS, Cloud Armor, etc.

> "Great suggestion! [Service Name] is currently on our roadmap. Our philosophy is to build 'high-fidelity' shims rather than shallow mocks, so it takes a bit more time to get the behavior exactly right.
>
> In the meantime, you can actually plug in your own Docker-based emulator using our Plugin system. Check the `docs/extending-minisky.md` for details, and we’d love a PR if you decide to build a shim for it!"

---

## 5. Bug Reports (Fidelity Issues)
**Use for:** When a user finds a discrepancy between MiniSky and real GCP.

> "Thanks for catching this! Our goal is 100% API parity with the GCP Discovery Documents. If MiniSky is behaving differently than the real cloud, that’s a bug we want to squash immediately.
>
> Could you please provide the specific API endpoint and the request/response payload? We’ll run it through our validator and get a fix out in the next release."

---

## 6. Data Engineering / BigQuery (DuckDB)
**Use for:** Users surprised by the speed or wondering how BigQuery works locally.

> "Glad you’re enjoying the speed! By using DuckDB as our analytical engine, we’re able to provide a BigQuery-compatible experience without the massive overhead of a full Java stack. It’s perfect for testing SQL transforms and Python/Node SDK logic without scanning terabytes of data in the cloud.
>
> Pro tip: You can even use the Dataproc integration to run PySpark jobs locally that read from 'gs://' buckets in MiniSky!"

---

## 7. Contribution / "How can I help?"
**Use for:** Developers wanting to get involved.

> "We’d love to have you! MiniSky is MIT Licensed and 100% community-driven. Whether it's adding a new service shim, improving the React dashboard, or just fixing a typo in the docs, every contribution helps.
>
> Check out our [Contributing Guide](https://github.com/qamarudeenm/minisky/blob/main/docs/contributing-guide.md) to get your local dev environment set up. Let's build the future of local GCP development together!"
