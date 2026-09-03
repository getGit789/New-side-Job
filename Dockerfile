FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X briefrelay/internal/web.Version=${VERSION}" -o /briefrelay ./cmd/briefrelay && mkdir -p /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /briefrelay /briefrelay
COPY --from=build --chown=nonroot:nonroot /data /data
ENV BRIEFRELAY_ADDR=0.0.0.0:8080 BRIEFRELAY_DATA_DIR=/data
VOLUME /data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/briefrelay"]
CMD ["serve"]
