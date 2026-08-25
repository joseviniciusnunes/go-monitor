# Go Monitor

Monitor de sistema leve escrito em Go que publica métricas de **temperatura da CPU**, **uso de memória RAM** e **uso de disco** via **MQTT** a cada intervalo configurável.

Projetado para rodar em um container Docker e coletar métricas do **host** (não do container), ideal para dispositivos como o **Orange Pi 4 Pro**.

## 📦 Funcionalidades

- 🌡️ **Temperatura da CPU** — lida diretamente do sysfs do host (`k10temp`/`coretemp`/`x86_pkg_temp`)
- 🧠 **Uso de memória RAM** — usado, total, percentual e valores formatados (MB/GB)
- 💾 **Uso de disco** — usado, total, percentual e valores formatados (MB/GB)
- 📡 **Publicação MQTT** — com reconexão automática e QoS 0
- 🐳 **Container Docker** — imagem mínima (`scratch`), binário estático
- 🏠 **Integração Home Assistant** — tópico compatível com o padrão de autodiscovery

## 📋 Pré-requisitos

- Docker e Docker Compose (para rodar em container)
- Go 1.24+ (para desenvolvimento local)
- Um broker MQTT (ex.: Mosquitto, Home Assistant)

## 🚀 Como usar

### Com Docker Compose

1. Clone o repositório:

   ```bash
   git clone <seu-repo-url>
   cd go-monitor
   ```

2. Ajuste as variáveis de ambiente no `docker-compose.yml` (broker, credenciais, tópico).

3. Suba o container:

   ```bash
   docker compose up -d --build
   ```

### Localmente (sem Docker)

```bash
go run .
```

## ⚙️ Configuração

Toda a configuração é feita por **variáveis de ambiente**:

| Variável | Padrão | Descrição |
| --- | --- | --- |
| `MQTT_BROKER` | `tcp://192.168.1.100:1883` | Endereço do broker MQTT |
| `MQTT_USER` | Usuário do broker |
| `MQTT_PASSWORD` | Senha do broker |
| `MQTT_TOPIC` | `/homeassistant/go-monitor/metrisc` | Tópico de publicação |
| `PUBLISH_INTERVAL` | `60` | Intervalo de publicação (segundos) |
| `LOG_LEVEL` | `error` | Nível de log: `error` ou `verbose` |
| `SYSFS_ROOT` | `/sys` | Raiz do sysfs do host (para temperatura) |
| `DISK_PATH` | `/` | Partição a monitorar |

### Níveis de log

- **`error`** (padrão) — exibe apenas erros
- **`verbose`** — exibe erros + informações (conexão, publicações, encerramento)

## 📊 Formato das métricas

As métricas são publicadas como **JSON** no tópico MQTT:

```json
{
  "cpu_temperature": 50.0,
  "ram_used": 7508373504,
  "ram_total": 33489620992,
  "ram_percent": 22,
  "ram_used_formatted": "7.00 GB",
  "ram_total_formatted": "31.19 GB",
  "disk_used": 372022906880,
  "disk_total": 490578284544,
  "disk_percent": 79,
  "disk_used_formatted": "346.47 GB",
  "disk_total_formatted": "456.90 GB",
  "hostname": "vinicius",
  "timestamp": 1787660212
}
```

| Campo | Tipo | Descrição |
| --- | --- | --- |
| `cpu_temperature` | float | Temperatura da CPU em °C |
| `ram_used` / `ram_total` | uint64 | Bytes usados/totais de RAM |
| `ram_percent` | int | Percentual de RAM usado |
| `ram_used_formatted` / `ram_total_formatted` | string | RAM formatada (MB/GB) |
| `disk_used` / `disk_total` | uint64 | Bytes usados/totais de disco |
| `disk_percent` | int | Percentual de disco usado |
| `disk_used_formatted` / `disk_total_formatted` | string | Disco formatado (MB/GB) |
| `hostname` | string | Nome do host |
| `timestamp` | int64 | Unix timestamp da coleta |

## 🐳 Docker

### Dockerfile

Build em **multi-stage**:

1. **Builder** — compila um binário estático com `CGO_ENABLED=0` e `-ldflags="-s -w"`
2. **Final** — imagem `scratch` mínima (~10MB), sem shell ou ferramentas

### docker-compose.yml

O container monta o sysfs e a raiz do host para coletar métricas reais:

```yaml
volumes:
  - /sys:/host/sys:ro   # temperatura da CPU
  - /:/host/root:ro     # uso do disco
network_mode: host      # métricas refletem o host
```

## 🏗️ Build para Orange Pi 4 Pro

O Orange Pi 4 Pro usa um SoC **RK3588S** (ARM64). O workflow do GitHub Actions (`build.yml`) gera e publica uma imagem `linux/arm64` no **GitHub Container Registry (GHCR)**.

Para rodar no Orange Pi:

```bash
docker pull ghcr.io/<seu-usuario>/go-monitor:latest
docker run -d \
  --name go-monitor \
  --network host \
  -e MQTT_BROKER="tcp://192.168.1.100:1883" \
  -e MQTT_USER="admin" \
  -e MQTT_PASSWORD="********" \
  -e MQTT_TOPIC="/homeassistant/go-monitor/metrics" \
  -e PUBLISH_INTERVAL="60" \
  -e LOG_LEVEL="error" \
  -v /sys:/host/sys:ro \
  -v /:/host/root:ro \
  ghcr.io/<seu-usuario>/go-monitor:latest
```

## 🧪 Testes

```bash
go test ./...
```

O teste valida que a leitura de temperatura da CPU retorna um valor dentro da faixa esperada (20–100°C).

## 📁 Estrutura do projeto

```text
.
├── .github/workflows/build.yml   # CI: build ARM64 + push GHCR
├── Dockerfile                    # Build multi-stage (scratch)
├── docker-compose.yml            # Orquestração do container
├── main.go                       # Código principal
├── main_test.go                  # Testes unitários
├── go.mod / go.sum               # Dependências Go
└── .dockerignore                 # Arquivos ignorados no build
```

## 🔧 Dependências

- [`eclipse/paho.mqtt.golang`](https://github.com/eclipse/paho.mqtt.golang) — cliente MQTT
- [`shirou/gopsutil`](https://github.com/shirou/gopsutil) — métricas de memória e disco

## 📄 Licença

Este projeto está sob a licença MIT. Veja o arquivo `LICENSE` para mais detalhes.
