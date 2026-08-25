package main

import "testing"

func TestReadCPUTemperature(t *testing.T) {
	temp := readCPUTemperature("/sys")
	if temp < 20 || temp > 100 {
		t.Errorf("Temperatura fora do esperado (%v°C). Sensor de CPU real deve estar entre 20-100°C", temp)
	}
}
