# 🚀 MiniSky: End-to-End Terraform Demo

This directory contains a complete Terraform configuration designed to showcase MiniSky's high-fidelity emulation of GCP services.

## 📋 What this demo shows:
1.  **High-Fidelity Storage:** Creating buckets with labels and metadata.
2.  **Pub/Sub Integration:** Managing topics and subscriptions.
3.  **BigQuery Analytics:** Provisioning datasets and tables with complex schemas.
4.  **Compute LROs:** Demonstrates how MiniSky handles Long-Running Operations (LROs) during VM creation, allowing Terraform to poll for status correctly.

## 🛠️ How to run the demo:

1.  **Start MiniSky:**
    Ensure the MiniSky daemon is running in a separate terminal:
    ```bash
    minisky start
    ```

2.  **Open the Dashboard:**
    Keep `http://localhost:8081` open in your browser to watch the resources appear in real-time.

3.  **Initialize Terraform:**
    ```bash
    cd examples/demo-launch
    terraform init
    ```

4.  **Apply the configuration:**
    ```bash
    terraform apply -auto-approve
    ```

5.  **Watch the magic:**
    Observe the Terminal output and the MiniSky Dashboard. You will see the Compute Instance transition through states just like in the real GCP console.

6.  **Cleanup:**
    ```bash
    terraform destroy -auto-approve
    ```
