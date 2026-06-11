FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /ovh-baremetal-ccm ./cmd/ovh-baremetal-ccm/

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /ovh-baremetal-ccm /ovh-baremetal-ccm
USER 65532:65532
ENTRYPOINT ["/ovh-baremetal-ccm"]
