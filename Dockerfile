FROM golang:1.27.1-alpine3.23 AS builder
WORKDIR /app

# Dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /gopal

# Stage 2
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /gopal /gopal

# Nonroot user
USER 1001:1001

CMD ["/gopal"]
