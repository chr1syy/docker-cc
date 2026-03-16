# Multi-stage build for Docker CC

# Frontend build
FROM node:22-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
COPY frontend/ ./
RUN npm ci && npm run build

# Backend build
FROM golang:1.24-alpine AS backend-build
ARG VERSION=dev
WORKDIR /app/backend
COPY backend/ ./
RUN GOTOOLCHAIN=local go build -ldflags "-X main.Version=${VERSION}" -o /app/server .

# Runtime image
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=backend-build /app/server /app/server
COPY --from=frontend-build /app/frontend/build /app/static
ENV STATIC_DIR=/app/static
EXPOSE 8080
ENTRYPOINT ["/app/server"]
