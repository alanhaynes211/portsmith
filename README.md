<p align="center">
  <img src=".github/logo.svg" alt="Portsmith Logo" width="200">
</p>

<h1 align="center">Portsmith</h1>

<p align="center">
  A blacksmith for ports: dynamically forges and tears down secure forwards on demand.
</p>

<p align="center">
  <a href="#-features">Features</a> •
  <a href="#-getting-started">Getting Started</a> •
  <a href="#-configuration">Configuration</a>
</p>

## ✨ Features

  * **Dynamic Port Forwarding:** Automatically establishes and maintains SSH port forwards through bastion hosts. Connections are established on-demand when traffic is received, minimizing resource usage.
  * **System Tray Application:** Run as a native macOS system tray app with start/stop controls and easy access to logs and configuration.
  * **Privileged Port Support:** Forward privileged ports (like SSH on port 22) without running as root.
  * **Automatic Loopback Aliases:** Creates local loopback aliases (e.g., `127.0.0.2`, `127.0.0.3`) for clean service separation.
  * **Automatic `/etc/hosts` Management:** Assigns custom hostnames (like `service.remote`) to forwarded services.
  * **Multiple Services:** Define and manage connections for multiple remote services. Forward a single port or entire range.
  * **SSH Agent Integration:** Uses your existing SSH agent for authentication, with support for custom identity agents (e.g., 1Password).
  * **Graceful Cleanup:** Automatically cleans up all resources on exit and removes stale resources on startup.

## 🚀 Getting Started

### Installation

#### Quick Install (Recommended)

Install the latest release with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/alanhaynes211/portsmith/main/install.sh | bash
```

This will automatically:
- Detect your platform (Intel or Apple Silicon)
- Download the latest release
- Install binaries to `/usr/local/bin`
- Configure sudoers for passwordless helper execution
- Set up the example config at `~/.config/portsmith/config.yaml`

#### Build from Source

If you prefer to build from source:

**Prerequisites:**
- macOS
- Go 1.21+
- `just` (for running build commands)

```bash
# Clone the repository
git clone https://github.com/alanhaynes211/portsmith.git
cd portsmith

# Build and install
just build-install
```

### Configure

Portsmith is configured using a YAML file located at `~/.config/portsmith/config.yaml`, which will be created during the installation.

### Run

Once configured, run Portsmith in either system tray mode (default) or CLI mode:

**System Tray Mode (Default)**

Simply run `portsmith` from a terminal. It will automatically daemonize and appear in your macOS menu bar:

```bash
# Add your SSH key (if not already loaded)
ssh-add ~/.ssh/id_rsa

# Run Portsmith (launches in system tray)
portsmith
```

The system tray icon provides controls to start/stop forwarding, open the config file, and view logs.

**CLI Mode**

For traditional terminal operation, use the `--cli` flag:

```bash
# Run in CLI mode
portsmith --cli
```

Portsmith will keep running in the foreground. Press `Ctrl+C` to gracefully shut down all connections and clean up network settings.

## 🔧 Configuration

The core of Portsmith is its configuration file. See the [config.example.yaml](https://github.com/alanhaynes211/portsmith/blob/main/config.example.yaml) for a detailed breakdown of all available options and examples.

A minimal configuration might look like this:

**`~/.config/portsmith/config.yaml`**

```
hosts:
  - local_ip: 127.0.0.2
    hostnames:
      - myapp.local
    remote_host: app.internal.example.com
    jump_host: bastion.example.com
    ports: [80, 443, "5432-5433"]
```

## ⚖️ License

This project is licensed under the MIT License.