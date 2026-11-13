# Estágio de build
FROM golang:1.24-alpine AS builder

# NÃO forçamos GOARCH/GOARM manualmente: o docker buildx executa este estágio
# sob QEMU para cada plataforma (linux/arm64, linux/arm/v7). Assim o go build
# nativo já gera o binário correto para cada arquitetura automaticamente,
# evitando erros como GOARM=v7 (valor inválido) e "exec format error".
WORKDIR /app

# Copia os módulos e baixa dependências (aproveita cache)
COPY go.mod go.sum ./
RUN go mod download

# Copia o código e compila um binário estático para a arquitetura alvo
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /go-monitor .

# Estágio final: imagem mínima
FROM scratch

COPY --from=builder /go-monitor /go-monitor

ENTRYPOINT ["/go-monitor"]