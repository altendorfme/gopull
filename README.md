# GoPull

![Gopher Pull](https://github.com/altendorfme/gopull/blob/main/gopher.png)

Automatic Git repository updates via deploy API in a Docker container.

## ✨ Features
- 🔌 Listens for deploy requests on port 15800
- 🔐 Secure API key verification
- 🔑 Optional SSH Deploy Keys generation (controlled by PRIVATE environment variable)
- 🔄 Performs `git pull --rebase` when deploy request received
- 🚫 Ignore specific files or folders during git pull (controlled by GITIGNORE environment variable)
- 🖥️ Works on:  `linux/amd64` (tel/AMD processors) / `linux/arm64` (Raspberry Pi, Apple M1/M2)

### 📦 Run with Docker

```bash
# PRIVATE mode (for private repositories with SSH authentication)
docker run -d \
  -p 15800:15800 \
  -e DEPLOY_KEY="your-secret-key" \
  -e PRIVATE="true" \
  -e GITIGNORE="wp-content/,wp-config.php" \
  -v ./work/app:/app \
  -v ./work/keys:/keys \
  --name gopull \
  ghcr.io/altendorfme/gopull:latest

# Non-PRIVATE mode (for public repositories or simple HTTPS cloning)
docker run -d \
  -p 15800:15800 \
  -e GIT_REPOURL="https://github.com/username/repository.git" \
  -e PRIVATE="false" \
  -e GITIGNORE="wp-content/,wp-config.php" \
  -v ./work/app:/app \
  --name gopull \
  ghcr.io/altendorfme/gopull:latest
```

### 🐙 Run with Docker Compose

```bash
curl -o ./docker-compose.yml https://raw.githubusercontent.com/altendorfme/gopull/main/docker-compose.yml

nano docker-compose.yml

docker-compose up -d
```

## 🔧 Setup

### 0️⃣ Choose your operation mode:

#### Non-PRIVATE Mode (Simple, Automatic Updates):
- Set `PRIVATE=false` (default)
- Set `GIT_REPOURL` to your repository's HTTPS URL
- No SSH keys are generated
- Repository is automatically checked for updates every minute
- No GitHub webhook setup required
- DEPLOY_KEY is optional (only needed if you still want webhook functionality)

#### PRIVATE Mode (For Private Repositories):
- Set `PRIVATE=true`
- Set `DEPLOY_KEY` for webhook authentication
- SSH keys are generated on startup
- Requires GitHub webhook setup
- Follow the steps below to add the Deploy Key to your GitHub repository

### 1️⃣ Get the SSH key from logs on first start (only when PRIVATE=true):

```bash
docker logs gopull
```

You'll see:
```
=== GitHub Deploy Key (Add this to your GitHub repository) ===
ssh-rsa AAAA...
============================================================
```

### 2️⃣ Add Deploy Key to GitHub

- Go to your repo → Settings → Deploy keys
- Click "Add deploy key"
- Paste the key and give it a title
- Check "Allow write access" if needed
- Click "Add key"

### 3️⃣ Set up Deploy URL (required for PRIVATE mode, optional for non-PRIVATE mode)

- Go to your repo → Settings → Webhooks
- Click "Add webhook"
- Payload URL: `http://your-server:15800/?deploy=your-secret-key` (Where `your-secret-key` is the same value you set for the `DEPLOY_KEY` environment variable.)
- Content type: `application/json`
- SSL verification: It's recommended to use a reverse proxy (like Nginx, Traefik, or Caddy) in front of GoPull, but if using port `15800` change to `Disable (not recommended)`
- Click "Add webhook"
## 📂 Volumes

- `/app/public`: Repository content
- `/app/public.git`: Git metadata (.git directory)
- `/keys`: Deploy SSH keys

## 🔧 Environment

| Variable | Description | Example |
|----------|-------------|---------|
| `DEPLOY_KEY` | Secret key for API validation | `your-secret-key` |
| `PRIVATE` | Enable SSH key generation for private repositories | `true` or `false` |
| `GIT_REPOURL` | Repository URL (required in non-PRIVATE mode) | `https://github.com/username/repository.git` |
| `GITIGNORE` | Comma-separated list of files/folders to ignore during git pull | `folder/,wp-config.php` |

## 🚫 Ignoring Files During Git Pull

The `GITIGNORE` environment variable allows you to specify files or folders that should be ignored during the git pull process. This is useful for preserving local changes to specific files that should not be overwritten by the remote repository.

Example:
```
GITIGNORE="wp-content/,wp-config.php"
```

This will ignore the "wp-content//" directory and the "wp-config.php" file during git pull operations.

> **Note:** While the `GITIGNORE` environment variable is useful for server-specific ignores, it is always recommended to use the `.gitignore` file within your repository for project-wide ignores. The `.gitignore` file is version-controlled and shared with all contributors, ensuring consistent behavior across different environments.