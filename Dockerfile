# Multi-stage build for Docker CC

# Frontend build
FROM node:22-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
COPY frontend/ ./
RUN npm ci && npm run build

# Backend build
FROM golang:1.23-alpine AS backend-build
WORKDIR /app/backend
COPY backend/ ./
RUN export PATH=$PATH:/usr/local/go/bin && go build -o /app/server .

# Runtime image
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend-build /app/server /app/server
COPY --from=frontend-build /app/frontend/build /app/static
ENV STATIC_DIR=/app/static
EXPOSE 8080
ENTRYPOINT ["/app/server"]
