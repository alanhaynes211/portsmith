default:
    @just --list

# Tidy go modules
tidy:
    go mod tidy
    cd helper && go mod tidy

# Build both portsmith and portsmith-helper
build: tidy
    @mkdir -p bin
    cd helper && go build -o ../bin/portsmith-helper
    go build -o bin/portsmith

# Build and install everything in one command
build-install: build install
    @echo "Build and installation complete!"

# Run portsmith as regular user
run:
    ./bin/portsmith

# Install portsmith to /usr/local/bin
install-portsmith:
    @echo "Installing portsmith to /usr/local/bin..."
    sudo cp bin/portsmith /usr/local/bin/
    sudo chmod 755 /usr/local/bin/portsmith
    @echo "Portsmith installed"

# Install helper and configure sudoers (requires sudo)
install-helper:
    @echo "Installing portsmith-helper to /usr/local/bin..."
    sudo cp bin/portsmith-helper /usr/local/bin/
    sudo chmod 755 /usr/local/bin/portsmith-helper
    @echo "Configuring sudoers..."
    @echo "$(whoami) ALL=(root) NOPASSWD: /usr/local/bin/portsmith-helper" | sudo tee /etc/sudoers.d/portsmith > /dev/null
    sudo chmod 0440 /etc/sudoers.d/portsmith
    @echo "Helper installed and sudoers configured"

# Install example config to ~/.config/portsmith/config.yaml if it doesn't exist
install-config:
    @mkdir -p ~/.config/portsmith
    @if [ ! -f ~/.config/portsmith/config.yaml ]; then \
        cp config.example.yaml ~/.config/portsmith/config.yaml; \
        echo "Example config installed to ~/.config/portsmith/config.yaml"; \
        echo "Edit this file to configure your hosts"; \
    else \
        echo "Config already exists at ~/.config/portsmith/config.yaml"; \
    fi

# Install everything (portsmith, helper, and config)
install: install-portsmith install-helper install-config
    @echo "Installation complete"

# Uninstall portsmith binary
uninstall-portsmith:
    sudo rm -f /usr/local/bin/portsmith
    @echo "Portsmith removed"

# Uninstall helper and remove sudoers config
uninstall-helper:
    sudo rm -f /usr/local/bin/portsmith-helper
    sudo rm -f /etc/sudoers.d/portsmith
    @echo "Helper and sudoers config removed"

# Remove config (WARNING: deletes your configuration)
uninstall-config:
    @if [ -f ~/.config/portsmith/config.yaml ]; then \
        rm -f ~/.config/portsmith/config.yaml; \
        rmdir ~/.config/portsmith 2>/dev/null || true; \
        echo "Config removed from ~/.config/portsmith/"; \
    else \
        echo "No config to remove"; \
    fi

# Uninstall everything (WARNING: includes config)
uninstall: uninstall-portsmith uninstall-helper uninstall-config
    @echo "Uninstallation complete"

# Clean up all portsmith network changes
cleanup:
    sudo /usr/local/bin/portsmith-helper remove-hosts
    sudo /usr/local/bin/portsmith-helper remove-aliases
    sudo /usr/local/bin/portsmith-helper remove-pf-redirects

# Remove built binaries
clean:
    rm -rf bin/
