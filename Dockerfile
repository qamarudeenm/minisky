# Stage 1: Build the UI
FROM node:20.18.0-alpine AS ui-builder
WORKDIR /app/ui
COPY ui/package*.json ./
RUN npm install
COPY ui/ ./
RUN npm run build

# Stage 2: Build the Go Binary
FROM golang:1.22.8-alpine AS binary-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/ui/dist /app/ui/dist
RUN go build -o minisky ./cmd/minisky

# Stage 3: Production Image
FROM alpine:latest
WORKDIR /app

# Install dependencies needed at runtime (Docker, Pack CLI)
RUN apk add --no-cache docker-cli curl

# Install Google Cloud Buildpacks (Pack CLI)
RUN curl -sSL https://github.com/buildpacks/pack/releases/download/v0.33.2/pack-v0.33.2-linux.tgz | tar -xzv -C /usr/local/bin pack

COPY --from=binary-builder /app/minisky /app/minisky

EXPOSE 8080 8081

# MiniSky needs access to the host's Docker socket
VOLUME /var/run/docker.sock

ENTRYPOINT ["/app/minisky", "start"]
