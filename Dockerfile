# syntax=docker/dockerfile:1

# Templates and CSS are embedded by go:embed, so the production image contains
# one executable and no Node.js runtime or copied asset directory.
FROM golang:1.26-alpine AS build
WORKDIR /build
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pitlane ./cmd/web

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pitlane /pitlane
EXPOSE 4000
ENTRYPOINT ["/pitlane"]
