FROM golang:1.15.4-buster

WORKDIR /app
COPY . /app
RUN go build .

EXPOSE 8080
CMD go run .
