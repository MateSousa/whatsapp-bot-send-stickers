FROM golang:1.19-alpine

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download 

COPY . .

CMD ["go", "run", "main.go"]