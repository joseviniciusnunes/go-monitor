# Estágio de build
FROM golang:1.24-alpine AS builder

# Arquitetura/variação alvo, definidas automaticamente pelo docker buildx
# via --platform. Para Orange Pi PC Plus (Allwinner H3, ARMv7): TARGETARCH=arm, TARGETVARIANT=v7
ARG TARGETARCH=amd64
ARG TARGETVARIANT=v7

WORKDIR /app

# Copia os módulos e baixa dependências (aproveita cache)
COPY go.mod go.sum ./
RUN go mod download

# Copia o código e compila um binário estático para a arquitetura alvo
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT} go build -ldflags="-s -w" -o /go-monitor .

# Estágio final: imagem mínima
FROM scratch

COPY --from=builder /go-monitor /go-monitor

ENTRYPOINT ["/go-monitor"]