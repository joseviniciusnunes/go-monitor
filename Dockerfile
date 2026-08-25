# Estágio de build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copia os módulos e baixa dependências (aproveita cache)
COPY go.mod go.sum ./
RUN go mod download

# Copia o código e compila um binário estático
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /go-monitor .

# Estágio final: imagem mínima
FROM scratch

COPY --from=builder /go-monitor /go-monitor

ENTRYPOINT ["/go-monitor"]