# Kaivue Appliance Installation Guide

## Overview

The Kaivue bootable installer produces a turnkey recording server appliance. It includes:

- Pre-compiled Kaivue server binary (linux/amd64 or linux/arm64)
- Hardware compatibility validation (runs automatically on first boot)
- First-boot setup wizard (10-minute guided configuration)
- systemd services for automatic startup and crash recovery

## System Requirements

| Tier | CPU Cores | RAM | Disk | Cameras |
|------|-----------|-----|------|---------|
| Mini | 2+ | 4 GB+ | 100 GB+ | 1-4 |
| Mid | 4+ | 8 GB+ | 500 GB+ | 5-16 |
| Enterprise | 8+ | 16 GB+ | 2 TB+ | 17-64 |

Minimum for installation: 2 cores, 4 GB RAM, 100 GB disk.

Supported OS: Debian 12 (Bookworm), Ubuntu 22.04/24.04, RHEL 9.

## Installation

### Option A: Tarball (recommended for existing Linux systems)

```bash
# Download the installer tarball for your architecture.
wget https://releases.kaivue.io/installer/kaivue-installer-v1.0.0-amd64.tar.gz

# Extract to root filesystem.
sudo tar -xzf kaivue-installer-v1.0.0-amd64.tar.gz -C /

# Enable services.
sudo systemctl daemon-reload
sudo systemctl enable kaivue-firstboot kaivue

# Reboot to trigger the first-boot wizard.
sudo reboot
```

### Option B: ISO image (for bare-metal appliances)

Flash the ISO to a USB drive:

```bash
sudo dd if=kaivue-installer-v1.0.0-amd64.iso of=/dev/sdX bs=4M status=progress
```

Boot from the USB drive. The installer will partition the disk, install the base OS, and launch the first-boot wizard.

## First-Boot Wizard

The wizard runs automatically on first boot (via `kaivue-firstboot.service`). It walks through 9 steps:

1. **Hardware Check** — validates CPU, RAM, disk, and network meet minimum requirements
2. **Master Key** — generates a 256-bit encryption key for database secrets
3. **Admin Account** — creates the initial administrator user
4. **Storage Path** — selects the recordings storage directory
5. **Network** — configures the listen address and ports
6. **Camera Discovery** — (optional) scans for ONVIF cameras on the local network
7. **Notifications** — (optional) configures email/push notification settings
8. **Remote Access** — (optional) sets up Tailscale/WireGuard remote access
9. **Cloud Pairing** — (optional) connects to Kaivue Cloud for fleet management

Steps 6-9 are optional and can be configured later from the admin UI.

### Resumability

The wizard saves progress after each step. If the system loses power or reboots mid-setup, the wizard resumes from the last completed step.

State is stored at `/var/lib/kaivue/state/wizard-state.json`.

### Completion

After the wizard completes, it creates `/etc/kaivue/wizard-complete` which:
- Disables the `kaivue-firstboot` service
- Enables the main `kaivue` service
- Starts the recording server

## Post-Installation

### Verify the server is running

```bash
sudo systemctl status kaivue
curl -s http://localhost:9997/v3/config/global/get | jq .
```

### Access the admin UI

Open `http://<server-ip>:9997` in a browser and log in with the admin account created during setup.

### Add cameras

Navigate to **Cameras > Add Camera** in the admin UI, or use the ONVIF auto-discovery feature.

## Building Custom Installer Images

```bash
# Build a tarball for amd64.
./scripts/build-installer.sh --arch amd64 --version v1.0.0 --mode tarball

# Build for arm64 (e.g., Raspberry Pi 4, Jetson Nano).
./scripts/build-installer.sh --arch arm64 --version v1.0.0 --mode tarball
```

See `scripts/build-installer.sh --help` for all options.

## Troubleshooting

### Wizard did not start

Check the first-boot service:
```bash
sudo systemctl status kaivue-firstboot
sudo journalctl -u kaivue-firstboot -e
```

### Hardware check failed

Run the validation script manually:
```bash
/usr/local/bin/kaivue-hw-validate
```

### Reset the wizard

```bash
sudo rm /etc/kaivue/wizard-complete
sudo rm /var/lib/kaivue/state/wizard-state.json
sudo systemctl restart kaivue-firstboot
```
