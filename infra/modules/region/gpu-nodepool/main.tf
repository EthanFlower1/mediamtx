# KAI-277: GPU node pool for NVIDIA Triton Inference Server.
# Adds g5.2xlarge / g5.4xlarge managed node groups with NVIDIA GPU plugin,
# cluster-autoscaler annotations, and Karpenter provisioner for overflow.

# ---------------------------------------------------------------------------
# GPU Node Group: inference (g5 instances with NVIDIA A10G)
# ---------------------------------------------------------------------------

resource "aws_eks_node_group" "gpu_inference" {
  cluster_name    = var.cluster_name
  node_group_name = "gpu-inference"
  node_role_arn   = var.node_role_arn
  subnet_ids      = var.subnet_ids

  instance_types = var.gpu_instance_types
  capacity_type  = "ON_DEMAND"
  ami_type       = "AL2_x86_64_GPU"

  scaling_config {
    desired_size = var.gpu_node_desired
    min_size     = var.gpu_node_min
    max_size     = var.gpu_node_max
  }

  update_config {
    max_unavailable = 1
  }

  labels = {
    "kaivue.io/workload"    = "gpu-inference"
    "kaivue.io/tier"        = "ml"
    "nvidia.com/gpu.present" = "true"
  }

  taint {
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  tags = merge(var.tags, {
    "k8s.io/cluster-autoscaler/enabled"                    = "true"
    "k8s.io/cluster-autoscaler/${var.cluster_name}"        = "owned"
    "k8s.io/cluster-autoscaler/node-template/label/kaivue.io/workload" = "gpu-inference"
    "k8s.io/cluster-autoscaler/node-template/taint/nvidia.com/gpu"     = "true:NoSchedule"
  })

  lifecycle {
    ignore_changes = [scaling_config[0].desired_size]
  }
}

# ---------------------------------------------------------------------------
# NVIDIA Device Plugin DaemonSet (Helm release via Terraform)
# ---------------------------------------------------------------------------

resource "helm_release" "nvidia_device_plugin" {
  name       = "nvidia-device-plugin"
  repository = "https://nvidia.github.io/k8s-device-plugin"
  chart      = "nvidia-device-plugin"
  version    = var.nvidia_device_plugin_version
  namespace  = "kube-system"

  set {
    name  = "tolerations[0].key"
    value = "nvidia.com/gpu"
  }
  set {
    name  = "tolerations[0].operator"
    value = "Exists"
  }
  set {
    name  = "tolerations[0].effect"
    value = "NoSchedule"
  }
  set {
    name  = "nodeSelector.nvidia\\.com/gpu\\.present"
    value = "true"
  }
}

# ---------------------------------------------------------------------------
# KEDA (Kubernetes Event-Driven Autoscaling)
# ---------------------------------------------------------------------------

resource "helm_release" "keda" {
  name             = "keda"
  repository       = "https://kedacore.github.io/charts"
  chart            = "keda"
  version          = var.keda_version
  namespace        = "keda"
  create_namespace = true

  set {
    name  = "serviceAccount.create"
    value = "true"
  }
}
