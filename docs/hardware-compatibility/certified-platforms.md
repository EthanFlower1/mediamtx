# Kaivue NVR Hardware Compatibility Matrix

## Hardware Tiers

| Tier | Cameras | CPU | RAM | Storage | Network | GPU (AI) |
|------|---------|-----|-----|---------|---------|----------|
| **Mini** | 4-8 | 2+ cores (Intel i3 / Ryzen 3) | 4 GB | 100 GB SSD/HDD | 1 GbE | Not required |
| **Mid** | 16-32 | 4+ cores (Intel i5 / Ryzen 5) | 8 GB | 500 GB SSD | 1 GbE (10 GbE recommended) | Optional (improves AI) |
| **Enterprise** | 64-128+ | 8+ cores (Intel i7/Xeon / Ryzen 7/9) | 16 GB+ | 2 TB+ SSD/RAID | 10 GbE | Recommended (NVIDIA) |

## Supported Operating Systems

| OS | Version | Architecture | Status |
|----|---------|-------------|--------|
| Ubuntu LTS | 22.04, 24.04 | amd64, arm64 | Certified |
| Debian | 12 (Bookworm) | amd64, arm64 | Certified |
| RHEL | 9.x | amd64 | Certified |
| Rocky Linux | 9.x | amd64 | Compatible |
| macOS | 14+ (Sonoma) | arm64 | Development only |
| Windows Server | 2022 | amd64 | Compatible (limited) |

## Known Compatible Hardware

### Mini Tier (4-8 cameras)
- **Intel NUC 13 Pro** (i3-1315U, 8 GB, 256 GB NVMe)
- **Beelink Mini S12 Pro** (N100, 16 GB, 500 GB)
- **Raspberry Pi 5** (8 GB) — arm64, testing only

### Mid Tier (16-32 cameras)
- **Intel NUC 13 Pro** (i7-1365U, 32 GB, 1 TB NVMe)
- **Dell OptiPlex Micro** (i5-13500T, 16 GB, 512 GB)
- **Supermicro SYS-E100** (Xeon W-1390, 32 GB, 2 TB)

### Enterprise Tier (64-128+ cameras)
- **Dell PowerEdge R360** (Xeon E-2478, 64 GB, 4x 2 TB RAID)
- **HP ProLiant DL20 Gen11** (Xeon E-2434, 32 GB, 2x 4 TB)
- **Supermicro SYS-5019A-FTN4** (Atom C3958, 64 GB, 8x HDD)
- **Custom NVR appliances** with NVIDIA T400/T1000 for AI inference

## GPU Support (AI Features)

AI features (person detection, face recognition, LPR, behavioral analytics) benefit significantly from GPU acceleration. Without a GPU, inference runs on CPU at reduced throughput.

| GPU | VRAM | Inference FPS (approx) | Status |
|-----|------|----------------------|--------|
| NVIDIA T400 | 4 GB | ~15 FPS | Supported |
| NVIDIA T1000 | 8 GB | ~30 FPS | Recommended |
| NVIDIA RTX A2000 | 12 GB | ~60 FPS | Enterprise |
| NVIDIA Jetson Orin Nano | 8 GB | ~20 FPS | Edge AI |

## Network Requirements

| Scenario | Bandwidth | Interface |
|----------|-----------|-----------|
| 4 cameras (1080p, 4 Mbps each) | 16 Mbps | 1 GbE sufficient |
| 16 cameras (1080p, 4 Mbps each) | 64 Mbps | 1 GbE sufficient |
| 32 cameras (4K, 12 Mbps each) | 384 Mbps | 1 GbE tight, 10 GbE recommended |
| 64+ cameras (mixed) | 500+ Mbps | 10 GbE required |

## Validation

Run the hardware validation script to check your system:

```bash
scripts/hw-validate.sh /path/to/recordings
```

The script outputs a JSON report with CPU, RAM, disk, network, and GPU status plus a tier classification. Exit code 0 means the system meets minimum requirements.
