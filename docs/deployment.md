# Image Build and Manual Deployment

This project deploys as three long-running services behind Docker Compose:

- `frontend`: Nginx serving the Vite build and proxying `/api` to the backend.
- `backend`: the Go API container.
- `postgres` and `redis`: stateful infrastructure with named Docker volumes.

## Production Server Setup

1. Install Docker and Docker Compose on the server.
2. Create a deployment directory, for example `/opt/lalalandshow`.
3. Copy `deploy/.env.prod.example` to `/opt/lalalandshow/.env`.
4. Fill every secret in `/opt/lalalandshow/.env`.

Copy these files to the server deployment directory:

- `docker-compose.prod.yml`
- `nginx.conf`
- `.env.prod.example`
- `remote-deploy.sh`

Do not store production secrets in GitHub source files. Keep them in the
server-side `.env` file.

## GitHub Images

The `Build Images` workflow uses `GITHUB_TOKEN` to publish images to GHCR:

- `ghcr.io/chenm0m/lalalandshow/backend:<tag>`
- `ghcr.io/chenm0m/lalalandshow/frontend:<tag>`

If GHCR packages are private, make sure the server can pull them. The simplest
option is to run this once on the server:

```bash
docker login ghcr.io
```

Use a GitHub PAT with package read access when prompted.

## Release Flow

Normal flow:

```text
push to main
  -> CI tests backend and builds frontend
  -> Build Images workflow builds images
  -> images are pushed to GHCR
  -> you manually pull and restart on the server
```

Manual image publishing is available from the `Build Images` workflow. Leave
`image_tag` empty to publish the current commit SHA, or provide a tag for a
named release.

On the server, set the image tag in `.release.env`:

```bash
cd /opt/lalalandshow
cat > .release.env <<'EOF'
BACKEND_IMAGE=ghcr.io/chenm0m/lalalandshow/backend
FRONTEND_IMAGE=ghcr.io/chenm0m/lalalandshow/frontend
IMAGE_TAG=latest
EOF
```

Then pull and restart:

```bash
cd /opt/lalalandshow
./remote-deploy.sh
```

For a specific commit or manual release tag, replace `IMAGE_TAG=latest` with
that exact tag.

## Fast Local Deploy

For on-site debugging, you can build images on your computer and stream them to
the server without waiting for GitHub Actions:

```bash
DEPLOY_HOST=your-server \
DEPLOY_USER=root \
DEPLOY_PATH=/opt/lalalandshow \
deploy/local-deploy.sh
```

Optional environment variables:

- `DEPLOY_PORT`: SSH port, defaults to `22`.
- `IMAGE_TAG`: image tag, defaults to `local-<git-sha>`.
- `BACKEND_IMAGE`: local backend image name.
- `FRONTEND_IMAGE`: local frontend image name.
- `SSH_OPTS`: extra SSH options, for example `-i ~/.ssh/deploy_key`.

This path is intentionally for fast debugging. For normal testing releases,
prefer pulling the GHCR images with `./remote-deploy.sh`.

## Rollback

On the server, edit `.release.env` and set `IMAGE_TAG` to an older known-good
tag. Then run:

```bash
cd /opt/lalalandshow
./remote-deploy.sh
```

## Local Production Smoke

After GitHub publishes images, you can test production Compose locally:

```bash
cp deploy/.env.prod.example deploy/.env
# edit deploy/.env with local-safe secrets and real image names
cd deploy
docker compose --env-file .env -f docker-compose.prod.yml up -d
```

Open `http://localhost:18088` and check `http://localhost:18088/healthz`.
