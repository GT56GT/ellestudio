# Build a static binary, then run it on a minimal non-root image.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ellie ./cmd/ellie && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ellie /app/ellie
COPY --from=build --chown=nonroot:nonroot /out/data /app/data
ENV DATA_DIR=/app/data
USER nonroot:nonroot
ENTRYPOINT ["/app/ellie"]
