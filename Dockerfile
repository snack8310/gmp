FROM golang:1.27-alpine AS api-build

WORKDIR /src/management
COPY management/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/campaign-api ./cmd/campaign-api

FROM alpine:3.24 AS api

RUN adduser -D -H -u 10001 app
COPY --from=api-build /out/campaign-api /app/campaign-api
USER app
EXPOSE 8082
CMD ["/app/campaign-api", "-addr", "0.0.0.0:8082"]

FROM node:24-alpine AS frontend

WORKDIR /app
COPY --chown=node:node frontend/demos/operations-flow/ ./
USER node
EXPOSE 8081
CMD ["node", "serve.mjs", "8081", "8082"]
