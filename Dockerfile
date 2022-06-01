FROM golang:1.18.2 AS builder
ARG GOPROXY
ARG GOPRIVATE
ARG GOOS=linux
ARG GOARCH=amd64

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
COPY *.go ./
RUN go mod download

RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -a -o tfspiegel *.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/tfspiegel .
USER 65532:65532

CMD ["/tfspiegel"]