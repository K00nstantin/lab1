FROM golang:1.22.2-alpine AS deps
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS tests
WORKDIR /app
COPY . .
# Fail the build if tests fail
RUN go test ./...

FROM deps AS builder
WORKDIR /app
COPY . .
RUN go build -o lab1

FROM alpine:latest AS runtime
RUN apk --no-cache add ca-certificates \
 && addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /app/lab1 .
USER appuser
EXPOSE 8080
CMD ["./lab1"]

# FROM golang:1.19-alpine AS builder

# WORKDIR /app

# COPY go.mod ./
# COPY go.sum ./

# RUN go mod download

# COPY *.go ./

# RUN go build -o lab1

# FROM alpine:latest

# RUN apk --no-cache add ca-certificates

# RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# WORKDIR /app

# COPY --from=builder /app/lab1 .

# USER appuser

# EXPOSE 8080

# CMD ["./lab1"]