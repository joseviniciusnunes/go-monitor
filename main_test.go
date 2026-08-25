package main

import "testing"

func TestReadCPUTemperature(t *testing.T) {
	temp := readCPUTemperature("/sys")
	if temp < 20 || temp > 100 {
		t.Errorf("Temperatura fora do esperado (%v°C). Sensor de CPU real deve estar entre 20-100°C", temp)
	}
}

func TestSanitizeID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"CPU Temperature", "CPU_Temperature"},
		{"ram_used", "ram_used"},
		{"Disk Percent", "Disk_Percent"},
		{"a b/c:d", "a_b_c_d"},
	}
	for _, c := range cases {
		if got := sanitizeID(c.in); got != c.want {
			t.Errorf("sanitizeID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
