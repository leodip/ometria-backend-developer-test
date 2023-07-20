FROM golang:1.20-bullseye as builder

RUN go install golang.org/dl/go1.20@latest \
    && go1.20 download

WORKDIR /build

COPY ./src .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v3 go build -v -o ./omimporter -tags timetzdata ./cmd/omimporter/main.go

FROM gcr.io/distroless/base-debian11

COPY --from=builder /build/omimporter /omimporter
COPY --from=builder /build/configs/config.json /config.json

ENTRYPOINT ["/omimporter", "-runIntervalInSeconds", "40"]
