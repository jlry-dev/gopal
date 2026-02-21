# Build Stage
FROM golang:1.25.7-alpine AS build

WORKDIR /app

COPY go.* ./

RUN go mod download

COPY . .

RUN go build -o ./bin/gopal

# Runtime
FROM scratch

ENV APP_ENV production

COPY --from=build /app/bin/gopal /gopal

CMD ["/gopal"]
