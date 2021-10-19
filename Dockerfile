FROM golang:1.16-alpine as builder

WORKDIR /app

LABEL maintainer="Tomasz Wlodarczyk <tomek.wlod@gmail.com>"

COPY go.mod .
COPY go.sum .

RUN apk --no-cache add ca-certificates
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o go-do

#############
# final stage
FROM scratch

WORKDIR /root/

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/go-do .

ENTRYPOINT ["./go-do"]