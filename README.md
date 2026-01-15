# Coffer

A CLI tool for managing application configuration with GCP Secret Manager integration.

Coffer lets you store configuration in version-controlled YAML files while keeping secrets secure in GCP Secret Manager. It supports environment-specific overlays, secret references, and injects resolved configuration as environment variables.

## Installation

```bash
go install github.com/sultano/coffer@latest
```

## Quick Start

```bash
# Initialize a new project
coffer init --gcp-project my-gcp-project

# Create your config
cat > config/base.yaml << 'EOF'
app:
  name: myapp
  port: 8080
database:
  host: localhost
  password: ${secret:db-password}
EOF

# Set a secret
coffer secret set db-password "supersecret"

# Run your app with config injected
coffer run -- node server.js
```

## Configuration

### Project Structure

```
myproject/
├── .coffer.yaml          # Project configuration
├── config/
│   ├── base.yaml         # Base configuration (required)
│   ├── dev.yaml          # Development overrides
│   ├── prod.yaml         # Production overrides
│   └── local.yaml        # Local overrides (gitignored)
```

### .coffer.yaml

```yaml
version: 1

config:
  path: ./config          # Config directory (default: ./config)
  base: base.yaml         # Base config file (default: base.yaml)

gcp:
  project: my-gcp-project
  secret_prefix: myapp-   # Optional: prefix for all secrets

environments:
  dev:
    gcp:
      project: my-gcp-project-dev
  prod:
    gcp:
      project: my-gcp-project-prod

env_mapping:              # Custom environment variable names
  database.host: DB_HOST
  database.password: DB_PASS

defaults:
  env: dev                # Default environment
```

### Config Overlay System

Configuration is merged in order (later files override earlier):

1. `base.yaml` - Base configuration
2. `{env}.yaml` - Environment-specific overrides
3. `local.yaml` - Local development overrides (not committed)

### Secret References

Reference secrets in your config using `${secret:name}` syntax:

```yaml
database:
  password: ${secret:db-password}           # Simple reference
  api_key: ${secret:api-key@2}              # Specific version
  other: ${secret:projects/other/secrets/x} # Cross-project reference
```

### Secret Prefix

Use `secret_prefix` to isolate secrets for multi-service projects:

```yaml
gcp:
  project: shared-dev
  secret_prefix: myservice-
```

With this config:
- `${secret:db-password}` fetches `myservice-db-password` from GCP
- `coffer secret set db-password` creates `myservice-db-password`
- `coffer secret list` shows only `myservice-*` secrets

## Commands

### Configuration

```bash
coffer resolve              # Output resolved config as JSON
coffer resolve -f yaml      # Output as YAML
coffer resolve -f dotenv    # Output as .env format

coffer get database.host    # Get a single config value

coffer run -- npm start     # Run command with config as env vars

coffer check                # Validate all secrets exist
coffer check --all          # Check all environments

coffer validate             # Validate config file syntax
```

### Secrets

```bash
coffer secret list                    # List all secrets
coffer secret get db-password         # Get a secret value
coffer secret set db-password "val"   # Create/update a secret
coffer secret set db-pass --from-file key.pem  # From file

coffer secret delete db-password      # Show what would be deleted
coffer secret delete db-password --yes # Actually delete

coffer secret unused                  # Find unreferenced secrets

coffer secret import .env             # Preview import from .env
coffer secret import .env --yes       # Import secrets from .env
```

### Project Setup

```bash
coffer init                           # Initialize new project
coffer init --gcp-project myproject   # With GCP project

coffer info                           # Show project info
```

## Environment Variables

Config keys are automatically converted to environment variables:

| Config Key | Environment Variable |
|------------|---------------------|
| `database.host` | `DATABASE_HOST` |
| `app.log_level` | `APP_LOG_LEVEL` |

Use `env_mapping` in `.coffer.yaml` to customize variable names.

## Authentication

Coffer uses Google Cloud Application Default Credentials (ADC):

```bash
# For local development
gcloud auth application-default login

# For CI/CD, use a service account
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

Required IAM roles:
- `roles/secretmanager.secretAccessor` - Read secrets
- `roles/secretmanager.admin` - Create/delete secrets (optional)

## Best Practices

1. **Add `local.yaml` to `.gitignore`** - Local overrides shouldn't be committed
2. **Use secret prefix** - Isolate secrets per service in shared projects
3. **Run `coffer check` in CI** - Catch missing secrets before deployment
4. **Use `coffer validate`** - Catch config errors early

## Examples

### Running in Different Environments

```bash
coffer run --env dev -- npm start
coffer run --env prod -- npm start
```

### Migrating from .env Files

```bash
# Preview what would be imported
coffer secret import .env

# Import all secrets
coffer secret import .env --yes
```

### CI/CD Pipeline

```yaml
# GitHub Actions example
- name: Check secrets
  run: coffer check --env prod

- name: Deploy
  run: coffer run --env prod -- ./deploy.sh
```

## License

MIT
