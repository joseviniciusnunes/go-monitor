package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Níveis de log suportados.
const (
	logLevelError   = "error"
	logLevelVerbose = "verbose"
)

// Config guarda as configurações carregadas de variáveis de ambiente.
type Config struct {
	Broker          string
	User            string
	Password        string
	Topic           string
	PublishInterval time.Duration
	// LogLevel controla a verbosidade: "error" (apenas erros) ou "verbose" (tudo).
	LogLevel string
	// SysfsRoot é o caminho raiz do sysfs do host (ex.: /host/sys).
	SysfsRoot string
	// DiskPath é o caminho da partição a monitorar (ex.: /host/root).
	DiskPath string
	// DiscoveryPrefix é o prefixo do MQTT discovery do Home Assistant (ex.: homeassistant).
	DiscoveryPrefix string
	// DeviceName é o nome do dispositivo exibido no Home Assistant.
	DeviceName string
}

// loadConfig lê as configurações de variáveis de ambiente.
func loadConfig() Config {
	return Config{
		Broker:          getEnv("MQTT_BROKER", "tcp://192.168.1.100:1883"),
		User:            getEnv("MQTT_USER", ""),
		Password:        getEnv("MQTT_PASSWORD", ""),
		Topic:           getEnv("MQTT_TOPIC", "/homeassistant/go-monitor/metrisc"),
		PublishInterval: time.Duration(getEnvInt("PUBLISH_INTERVAL", 60)) * time.Second,
		LogLevel:        getEnv("LOG_LEVEL", logLevelError),
		SysfsRoot:       getEnv("SYSFS_ROOT", "/sys"),
		DiskPath:        getEnv("DISK_PATH", "/"),
		DiscoveryPrefix: getEnv("DISCOVERY_PREFIX", "homeassistant"),
		DeviceName:      getEnv("DEVICE_NAME", "Go Monitor"),
	}
}

// verbose indica se o nível de log atual é "verbose".
func (cfg Config) verbose() bool {
	return cfg.LogLevel == logLevelVerbose
}

// logInfo registra mensagens informativas somente no modo verbose.
func (cfg Config) logInfo(format string, v ...any) {
	if cfg.verbose() {
		log.Printf(format, v...)
	}
}

// logError registra erros sempre, independentemente do nível de log.
func (cfg Config) logError(format string, v ...any) {
	log.Printf("ERRO: "+format, v...)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Metrics agrupa as métricas publicadas via MQTT.
type Metrics struct {
	CPUTemperature     float64 `json:"cpu_temperature"`
	RAMUsed            uint64  `json:"ram_used"`
	RAMTotal           uint64  `json:"ram_total"`
	RAMPercent         int     `json:"ram_percent"`
	RAMUsedFormatted   string  `json:"ram_used_formatted"`
	RAMTotalFormatted  string  `json:"ram_total_formatted"`
	DiskUsed           uint64  `json:"disk_used"`
	DiskTotal          uint64  `json:"disk_total"`
	DiskPercent        int     `json:"disk_percent"`
	DiskUsedFormatted  string  `json:"disk_used_formatted"`
	DiskTotalFormatted string  `json:"disk_total_formatted"`
	Hostname           string  `json:"hostname"`
	Timestamp          int64   `json:"timestamp"`
}

func main() {
	cfg := loadConfig()

	client := newMQTTClient(cfg)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		cfg.logError("Erro ao conectar ao broker MQTT: %v", token.Error())
		log.Fatal("Encerrando por falha de conexão")
	}
	defer client.Disconnect(250)

	// Publica o discovery do Home Assistant para os sensores.
	publishDiscovery(client, cfg)

	// Publica imediatamente e depois a cada intervalo configurado.
	publish(client, cfg)

	ticker := time.NewTicker(cfg.PublishInterval)
	defer ticker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			publish(client, cfg)
		case <-sig:
			cfg.logInfo("Encerrando...")
			return
		}
	}
}

// newMQTTClient configura e retorna um cliente MQTT com reconexão automática.
func newMQTTClient(cfg Config) mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID("go-monitor-" + sanitizeID(cfg.DeviceName)).
		SetUsername(cfg.User).
		SetPassword(cfg.Password).
		SetAutoReconnect(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectTimeout(5 * time.Second)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		cfg.logInfo("Conectado ao broker MQTT")
	})

	return mqtt.NewClient(opts)
}

// publish coleta as métricas e as publica no tópico MQTT.
func publish(client mqtt.Client, cfg Config) {
	m := collectMetrics(cfg)

	payload, err := json.Marshal(m)
	if err != nil {
		cfg.logError("Erro ao serializar métricas: %v", err)
		return
	}

	token := client.Publish(cfg.Topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		cfg.logError("Erro ao publicar métricas: %v", token.Error())
		return
	}
	cfg.logInfo("Métricas publicadas em %s", cfg.Topic)
}

// publishDiscovery publica os configs de discovery do Home Assistant
// para cada sensor, permitindo que o HA os descubra automaticamente.
func publishDiscovery(client mqtt.Client, cfg Config) {
	// Identificador único do dispositivo (usa o nome + hostname).
	deviceID := "go-monitor-" + sanitizeID(hostnameOr("host"))

	// Cada sensor é um par (nome, chave do JSON nas métricas) e o template
	// do valor formatado exibido nos atributos.
	sensors := []struct {
		name      string
		key       string
		unit      string
		icon      string
		formatted string // template do valor formatado (ex.: "{{ value_json.ram_used_formatted }}")
	}{
		{"CPU Temperature", "cpu_temperature", "°C", "mdi:thermometer", "{{ value_json.cpu_temperature }}°C"},
		{"RAM Used", "ram_used", "B", "mdi:memory", "{{ value_json.ram_used_formatted }}"},
		{"RAM Total", "ram_total", "B", "mdi:memory", "{{ value_json.ram_total_formatted }}"},
		{"RAM Percent", "ram_percent", "%", "mdi:memory", "{{ value_json.ram_percent }}%"},
		{"Disk Used", "disk_used", "B", "mdi:harddisk", "{{ value_json.disk_used_formatted }}"},
		{"Disk Total", "disk_total", "B", "mdi:harddisk", "{{ value_json.disk_total_formatted }}"},
		{"Disk Percent", "disk_percent", "%", "mdi:harddisk", "{{ value_json.disk_percent }}%"},
	}

	for _, s := range sensors {
		// Tópico de discovery: <prefix>/sensor/<device_id>/<object_id>/config
		objectID := sanitizeID(s.name)
		topic := fmt.Sprintf("%s/sensor/%s/%s/config", cfg.DiscoveryPrefix, deviceID, objectID)

		payload := map[string]any{
			"name":                s.name,
			"unique_id":           deviceID + "_" + objectID,
			"state_topic":         cfg.Topic,
			"value_template":      "{{ value_json." + s.key + " }}",
			"unit_of_measurement": s.unit,
			"icon":                s.icon,
			// Necessário para que o HA registre estatísticas de longo prazo.
			"state_class": "measurement",
			// Atributos: valor formatado e data da última leitura.
			"json_attributes_topic":    cfg.Topic,
			"json_attributes_template": fmt.Sprintf("{\"formatted\": \"%s\", \"last_reading\": \"{{ value_json.timestamp | timestamp_local }}\"}", s.formatted),
			"device": map[string]any{
				"identifiers":  []string{deviceID},
				"name":         cfg.DeviceName,
				"manufacturer": "go-monitor",
				"model":        "Orange Pi 4 Pro",
			},
		}

		data, err := json.Marshal(payload)
		if err != nil {
			cfg.logError("Erro ao serializar discovery de %s: %v", s.name, err)
			continue
		}

		token := client.Publish(topic, 0, true, data)
		token.Wait()
		if token.Error() != nil {
			cfg.logError("Erro ao publicar discovery de %s: %v", s.name, token.Error())
			continue
		}
		cfg.logInfo("Discovery publicado para %s em %s", s.name, topic)
	}
}

// hostnameOr retorna o hostname do sistema ou um fallback.
func hostnameOr(fallback string) string {
	if hn, err := os.Hostname(); err == nil && hn != "" {
		return hn
	}
	return fallback
}

// sanitizeID remove caracteres inválidos para IDs do Home Assistant.
func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// collectMetrics coleta temperatura da CPU, uso de RAM e uso de disco.
func collectMetrics(cfg Config) Metrics {
	m := Metrics{Timestamp: time.Now().Unix()}

	if hn, err := os.Hostname(); err == nil {
		m.Hostname = hn
	}

	m.CPUTemperature = readCPUTemperature(cfg.SysfsRoot)
	if cfg.verbose() {
		logThermalZones(cfg.SysfsRoot + "/class/thermal")
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		m.RAMUsed = vm.Used
		m.RAMTotal = vm.Total
		m.RAMPercent = int(vm.UsedPercent)
		m.RAMUsedFormatted = formatBytes(vm.Used)
		m.RAMTotalFormatted = formatBytes(vm.Total)
	}

	if du, err := disk.Usage(cfg.DiskPath); err == nil {
		m.DiskUsed = du.Used
		m.DiskTotal = du.Total
		m.DiskPercent = int(du.UsedPercent)
		m.DiskUsedFormatted = formatBytes(du.Used)
		m.DiskTotalFormatted = formatBytes(du.Total)
	}

	return m
}

// formatBytes converte bytes em uma representação legível (KB/MB/GB).
func formatBytes(b uint64) string {
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	const kb = 1024
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// readCPUTemperature lê a temperatura da CPU diretamente do sysfs do host.
// Retorna 0 se o sensor não estiver disponível.
func readCPUTemperature(sysfsRoot string) float64 {
	// 1) Sensores de CPU em hwmon: k10temp (AMD) e coretemp (Intel).
	if t := findSensorTemp(sysfsRoot+"/class/hwmon", "k10temp"); t > 0 {
		return t
	}
	if t := findSensorTemp(sysfsRoot+"/class/hwmon", "coretemp"); t > 0 {
		return t
	}

	// 2) Zonas térmicas de CPU/SOC (Intel x86_pkg_temp, Rockchip RK3588, etc.).
	thermalDir := sysfsRoot + "/class/thermal"
	for _, zoneType := range []string{
		"x86_pkg_temp",       // Intel
		"soc_thermal",        // Rockchip RK3588 (Orange Pi 4 Pro)
		"cpu_thermal",        // Rockchip RK3588
		"cpub_thermal_zone",  // Rockchip RK3588 (Orange Pi 4 Pro) - CPU big cores
		"cpul_thermal_zone",  // Rockchip RK3588 (Orange Pi 4 Pro) - CPU little cores
		"bigcore0_thermal",   // Rockchip RK3588
		"bigcore1_thermal",   // Rockchip RK3588
		"littlecore_thermal", // Rockchip RK3588
		"center_thermal",     // Rockchip RK3588
		"gpu_thermal",        // Rockchip RK3588
		"gpu_thermal_zone",   // Orange Pi 4 Pro
		"npu_thermal",        // Rockchip RK3588
		"npu_thermal_zone",   // Orange Pi 4 Pro
	} {
		if t := findThermalZoneTemp(thermalDir, zoneType); t > 0 {
			return t
		}
	}

	// 3) Fallback genérico: primeira zona térmica com temperatura válida.
	if t := firstThermalZoneTemp(thermalDir); t > 0 {
		return t
	}
	return 0
}

// logThermalZones lista as zonas térmicas disponíveis (para diagnóstico).
func logThermalZones(thermalDir string) {
	entries, err := os.ReadDir(thermalDir)
	if err != nil {
		log.Printf("ERRO: não foi possível ler %s: %v", thermalDir, err)
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		zone := thermalDir + "/" + e.Name()

		// O sysfs segue symlinks; para o alvo para verificar se é diretório.
		info, err := os.Stat(zone)
		if err != nil || !info.IsDir() {
			continue
		}
		typeData, err := os.ReadFile(zone + "/type")
		if err != nil {
			continue
		}
		temp := readTempMillidegrees(zone + "/temp")
		log.Printf("Zona térmica %s: tipo=%s temp=%.1f°C", e.Name(), strings.TrimSpace(string(typeData)), temp)
	}
}

// findSensorTemp procura em todos os hwmons um sensor com o nome dado
// (ex.: k10temp, coretemp) e retorna sua temperatura em graus Celsius.
func findSensorTemp(hwmonDir, sensorName string) float64 {
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		return 0
	}

	for _, e := range entries {
		hwmon := hwmonDir + "/" + e.Name()

		// O sysfs usa symlinks; verifica o alvo, não o link.
		info, err := os.Stat(hwmon)
		if err != nil || !info.IsDir() {
			continue
		}

		nameData, err := os.ReadFile(hwmon + "/name")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(nameData)) != sensorName {
			continue
		}

		// Dentro do sensor correto, lê o primeiro tempN_input.
		inputs, err := os.ReadDir(hwmon)
		if err != nil {
			continue
		}
		for _, in := range inputs {
			name := in.Name()
			if !strings.HasPrefix(name, "temp") || !strings.HasSuffix(name, "_input") {
				continue
			}
			return readTempMillidegrees(hwmon + "/" + name)
		}
	}
	return 0
}

// findThermalZoneTemp procura uma zona térmica com o tipo dado
// (ex.: x86_pkg_temp, soc_thermal) e retorna sua temperatura em °C.
func findThermalZoneTemp(thermalDir, zoneType string) float64 {
	entries, err := os.ReadDir(thermalDir)
	if err != nil {
		return 0
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		zone := thermalDir + "/" + e.Name()

		// Segue symlinks (no sysfs, thermal_zoneN são links para diretórios).
		info, err := os.Stat(zone)
		if err != nil || !info.IsDir() {
			continue
		}

		typeData, err := os.ReadFile(zone + "/type")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(typeData)) != zoneType {
			continue
		}
		return readTempMillidegrees(zone + "/temp")
	}
	return 0
}

// firstThermalZoneTemp retorna a temperatura da primeira zona térmica
// que reportar um valor válido (fallback genérico).
func firstThermalZoneTemp(thermalDir string) float64 {
	entries, err := os.ReadDir(thermalDir)
	if err != nil {
		return 0
	}

	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		zone := thermalDir + "/" + e.Name()

		// O sysfs usa symlinks; segue o alvo, não o link.
		info, err := os.Stat(zone)
		if err != nil || !info.IsDir() {
			continue
		}
		if t := readTempMillidegrees(zone + "/temp"); t > 0 {
			return t
		}
	}
	return 0
}

// readTempMillidegrees lê um valor de temperatura em miligraus
// (ex.: 45000 = 45°C) e retorna em graus Celsius.
func readTempMillidegrees(path string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	raw := strings.TrimSpace(string(data))
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	// Alguns sensores reportam em miligraus (ex.: 45000 = 45°C).
	if val > 1000 {
		val /= 1000
	}
	return val
}
