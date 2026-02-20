FROM golang:1.25.7-alpine

WORKDIR ./app

COPY go.* ./

RUN go mod download

COPY . .

RUN go build -o ./bin/gopal

CMD ["./bin/./gopal"]
