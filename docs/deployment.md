# Image Build and Manual Deployment

This project deploys as one standalone application image:

- `app`: one image containing the Vite frontend, Nginx, the Go backend,
  PostgreSQL, Redis, and migrations.
- Runtime data is stored under `/data`, so mount a Docker volume there.

## Production Server Setup

1. Install Docker on the server.
2. Pull the image.
3. Run one container with a persistent `/data` volume.

Minimal deployment:

```bash
docker pull ghcr.io/chenm0m/lalalandshow/app:latest
docker run -d \
  --name lalalandshow \
  --restart unless-stopped \
  -p 18088:8080 \
  -v lalalandshow-data:/data \
  ghcr.io/chenm0m/lalalandshow/app:latest
```

Default test accounts:

- Admin: `admin` / `admin`
- Checker: `checker` / `checker`

For real use, override secrets and passwords with `-e` flags or an env file.

## GitHub Images

The `Build Images` workflow uses `GITHUB_TOKEN` to publish one GHCR image:

- `ghcr.io/chenm0m/lalalandshow/app:<tag>`

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
  -> you manually pull and restart the container on the server
```

Manual image publishing is available from the `Build Images` workflow. Leave
`image_tag` empty to publish the current commit SHA, or provide a tag for a
named release.

To update the server:

```bash
docker pull ghcr.io/chenm0m/lalalandshow/app:latest
docker stop lalalandshow || true
docker rm lalalandshow || true
docker run -d \
  --name lalalandshow \
  --restart unless-stopped \
  -p 18088:8080 \
  -v lalalandshow-data:/data \
  ghcr.io/chenm0m/lalalandshow/app:latest
```

For a specific commit or manual release tag, replace `latest` with that exact
tag.

Docker Compose is still available as an optional wrapper via
`deploy/docker-compose.prod.yml`, but it is no longer required for the simple
test deployment path.

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
- `APP_IMAGE`: local app image name.
- `SSH_OPTS`: extra SSH options, for example `-i ~/.ssh/deploy_key`.

This path is intentionally for fast debugging. For normal testing releases,
prefer pulling the GHCR image and recreating the container.

## Rollback

Use the same run command with an older known-good tag:

```bash
docker pull ghcr.io/chenm0m/lalalandshow/app:<old-tag>
docker stop lalalandshow || true
docker rm lalalandshow || true
docker run -d \
  --name lalalandshow \
  --restart unless-stopped \
  -p 18088:8080 \
  -v lalalandshow-data:/data \
  ghcr.io/chenm0m/lalalandshow/app:<old-tag>
```

## Local Production Smoke

After GitHub publishes images, you can test locally:

```bash
docker run -d --name lalalandshow-smoke -p 18088:8080 -v lalalandshow-smoke-data:/data ghcr.io/chenm0m/lalalandshow/app:latest
```

Open `http://localhost:18088` and check `http://localhost:18088/healthz`.
