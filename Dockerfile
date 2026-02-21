# Build Stage
FROM golang:1.25.7-alpine AS build

WORKDIR /app

RUN adduser -D -u 1001 nonroot

COPY go.* ./

RUN go mod download

COPY . .

RUN go build -o ./bin/gopal

# Runtime
FROM scratch

ENV APP_ENV production

COPY --from=build /etc/passwd /etc/passwd

COPY --from=build /app/bin/gopal /gopal

USER nonroot

CMD ["/gopal"]
