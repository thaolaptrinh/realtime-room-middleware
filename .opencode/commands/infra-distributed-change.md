---
description: Change distributed K3s, Redis, KEDA, ECR, or Kubernetes manifests
---

Distributed infra change: $ARGUMENTS

Rules:
1. Do not run kubectl apply unless explicitly approved.
2. Do not run terraform apply unless explicitly approved.
3. Update deployments/distributed-k3s and runbooks.
4. Validate Redis/KEDA/Gateway/Game node interactions.
5. Include rollback plan.
