# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html vite.config.ts tsconfig*.json ./
COPY public ./public
COPY src ./src
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine AS backend
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
COPY --from=frontend /src/dist ./webui/dist
# Run embedded-resource tests on the builder architecture before cross-compiling.
RUN go test -tags embed_frontend ./... && go vet -tags embed_frontend ./...
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags embed_frontend -trimpath -o /out/server .

FROM alpine:3.23
WORKDIR /app
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose
COPY --from=backend /out/server /app/server
ENV PORT=80
EXPOSE 80
ENTRYPOINT ["/app/server"]
