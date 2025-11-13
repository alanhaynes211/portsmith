<p align="center">
  <img src=".github/logo.svg" alt="Portsmith Logo" width="200">
</p>

<p align="center">
  Forges and tears down SSH port forwards on demand.
</p>

<p align="center">
  <a href="#-features">Features</a> •
  <a href="#-getting-started">Getting Started</a> •
  <a href="#-configuration">Configuration</a> •
  <a href="#-how-it-works">How It Works</a>
</p>

## ✨ Features

  * **On Demand Port Forwarding:** Automatically establishes and maintains SSH port forwards through bastion hosts. Connections are established on-demand when traffic is received, minimizing resource usage.
  * **System Tray Application:** Run as a native macOS system tray app with start/stop controls and easy access to logs and configuration.
  * **Privileged Port Support:** Forward privileged ports (like SSH on port 22).
  * **Automatic Loopback Aliases:** Creates local loopback aliases (e.g., `127.0.0.2`, `127.0.0.3`) with `/etc/hosts` entries for clean service separation.
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

Portsmith is configured using a YAML file located at `~/.config/portsmith/config.yaml`, which will be created during the installation. Below is a minimal example.

```yaml
hosts:
  - local_ip: 127.0.0.2
    remote_host: app.internal.example.com
    jump_host: bastion.example.com
    ports: [80, "8443-8444"]
```

**Note:** When `remote_host` is a domain name (not an IP address), `hostnames` automatically defaults to the value of `remote_host`. In the example above, Portsmith will create an `/etc/hosts` entry mapping `127.0.0.2` to `app.internal.example.com`. You can override this by explicitly specifying `hostnames` if you prefer different local names.


### Run

Once configured, run Portsmith and it will appear in your macOS menu bar:

```bash
# Run Portsmith (launches in system tray)
portsmith
```

The terminal will remain open but logs are redirected to `~/Library/Logs/Portsmith/portsmith.log`. The system tray icon provides controls to:
- Start/stop forwarding
- Open the config file
- View logs
- **Enable/disable "Start at Login"** to automatically launch Portsmith when you log in
- Quit the application

Press `Ctrl+C` in the terminal to gracefully shut down all connections and clean up network settings.

### SSH Key Management

Portsmith requires access to your SSH keys for authentication. There are several ways to manage this:

**Option 1: macOS Keychain (Recommended)**

Store your SSH key passphrase in macOS Keychain for automatic loading:

```bash
# One-time setup: add your key to macOS Keychain
ssh-add --apple-use-keychain ~/.ssh/id_rsa

# Configure SSH to use keychain (optional, improves compatibility)
cat >> ~/.ssh/config << 'EOF'
Host *
  UseKeychain yes
  AddKeysToAgent yes
EOF
```

Portsmith will automatically load keys from the Keychain when needed. After a reboot, keys are loaded on-demand without prompting.

**Option 2: Identity Agent (1Password, etc.)**

Use an identity agent like 1Password's SSH agent for seamless authentication:

```yaml
# In your config.yaml
identity_agent: ~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock
```

The agent handles authentication via GUI prompts or biometric unlock.

**Option 3: Manual SSH Agent**

If running portsmith manually from a terminal:

```bash
# Add your SSH key to the agent
ssh-add ~/.ssh/id_rsa

# Then run portsmith
portsmith
```

**Note:** For automatic startup at login, use Option 1 or Option 2. Option 3 requires manual setup after each reboot.

## 🔍 How It Works

### 1. Loopback Interface Aliases

For each configured host, Portsmith creates a loopback interface alias (e.g., `127.0.0.2`, `127.0.0.3`) using the `ifconfig` command. This allows your Mac to have multiple local IP addresses beyond the standard `127.0.0.1`, providing clean separation between different services.

```bash
# Example: Create alias for 127.0.0.2
sudo ifconfig lo0 alias 127.0.0.2 up
```

### 2. `/etc/hosts` Entries

Portsmith adds entries to your `/etc/hosts` file to map friendly hostnames to these loopback aliases.

```bash
# Example entry added to /etc/hosts
127.0.0.2 myapp.local # portsmith-dynamic-forward
```

All entries are marked with `# portsmith-dynamic-forward` for easy identification and cleanup.

### 3. Privileged Port Forwarding with PF
One of Portsmith's key features is supporting privileged ports (ports < 1024, like SSH on port 22) without requiring Portsmith itself to run as root.

```
User connects to:  127.0.0.2:22 (privileged)
       ↓
PF redirects to:   127.0.0.2:10022 (unprivileged)
       ↓
Portsmith listens: 127.0.0.2:10022
```

The `portsmith-helper` binary (which runs with sudo privileges via the sudoers configuration) manages these PF redirect rules by:
- Adding redirect rules to `/etc/pf.anchors/portsmith`
- Configuring the `portsmith` anchor in `/etc/pf.conf`
- Loading the rules using `pfctl`

### 4. Dynamic SSH Tunneling

When traffic arrives on a forwarded port, Portsmith:
1. Establishes an SSH connection to the jump host (bastion)
2. Creates a local port forward from the aliased loopback address to the remote service
3. Maintains the connection while in use
4. Tears down the connection when idle

### The `portsmith-helper` Binary

The helper binary performs privileged operations that require root access. It's configured in sudoers to allow passwordless execution of specific commands:

| Command                               | Description                                     | Example                                                  |
| ------------------------------------- | ----------------------------------------------- | -------------------------------------------------------- |
| `add-alias <ip>`                      | Add loopback interface alias                    | `portsmith-helper add-alias 127.0.0.2`                   |
| `remove-alias <ip>`                   | Remove specific loopback alias                  | `portsmith-helper remove-alias 127.0.0.2`                |
| `remove-aliases`                      | Remove all 127.0.0.x aliases (except 127.0.0.1) | `portsmith-helper remove-aliases`                        |
| `add-host <ip> <hostname>`            | Add /etc/hosts entry                            | `portsmith-helper add-host 127.0.0.2 myapp.local`        |
| `remove-host <ip> <hostname>`         | Remove specific /etc/hosts entry                | `portsmith-helper remove-host 127.0.0.2 myapp.local`     |
| `remove-hosts`                        | Remove all portsmith /etc/hosts entries         | `portsmith-helper remove-hosts`                          |
| `add-pf-redirect <ip> <from> <to>`    | Add PF port redirect                            | `portsmith-helper add-pf-redirect 127.0.0.2 22 10022`    |
| `remove-pf-redirect <ip> <from> <to>` | Remove specific PF redirect                     | `portsmith-helper remove-pf-redirect 127.0.0.2 22 10022` |
| `remove-pf-redirects`                 | Remove all portsmith PF redirects               | `portsmith-helper remove-pf-redirects`                   |


### Graceful Cleanup

When Portsmith exits (via `Ctrl+C` or the system tray "Stop" action), it automatically:
- Closes all SSH connections
- Removes all loopback aliases it created
- Removes all /etc/hosts entries
- Removes all PF redirect rules

On startup, Portsmith also cleans up any stale resources from previous runs that didn't shut down cleanly.

## ⚖️ License

This project is licensed under the MIT License.