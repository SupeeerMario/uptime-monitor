FROM golang:1.27-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go install github.com/air-verse/air@v1.62.0

RUN go mod download

COPY . .

EXPOSE 8000

CMD [ "air" ]