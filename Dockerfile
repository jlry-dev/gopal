FROM golang:1.25.7-alpine

WORKDIR ./app

RUN apk update

COPY . .

CMD ["go","run","."]
