// Package smart extracts a disk's identity and health from SMART reports:
// smartctl text output (-i, -a, -x), smartctl JSON output (-j) and
// CrystalDiskInfo text exports.
package smart

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoData means the input doesn't look like a SMART report.
var ErrNoData = errors.New("nessun dato SMART riconosciuto: incolla l'output di smartctl (-a, -x o -j) o di CrystalDiskInfo")

type Info struct {
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	Firmware     string `json:"firmware"`
	Capacity     int64  `json:"capacity"`
	Kind         string `json:"kind"`         // HDD, SSD, NVMe or "" when unknown
	Health       string `json:"health"`       // as reported: PASSED, FAILED, OK, Good…
	PowerOnHours int64  `json:"powerOnHours"` // 0 when unknown
}

func Parse(data []byte) (Info, error) {
	data = bytes.TrimPrefix(bytes.TrimSpace(data), []byte("\xef\xbb\xbf"))
	var info Info
	if len(data) > 0 && data[0] == '{' {
		if err := parseJSON(data, &info); err != nil {
			return info, errors.New("JSON di smartctl non valido: " + err.Error())
		}
	} else {
		parseText(data, &info)
	}
	if info.Serial == "" && info.Model == "" && info.Capacity == 0 {
		return info, ErrNoData
	}
	return info, nil
}

type jsonReport struct {
	Device struct {
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	} `json:"device"`
	ModelName       string `json:"model_name"`
	ScsiVendor      string `json:"scsi_vendor"`
	ScsiProduct     string `json:"scsi_product"`
	ScsiModelName   string `json:"scsi_model_name"`
	SerialNumber    string `json:"serial_number"`
	FirmwareVersion string `json:"firmware_version"`
	ScsiRevision    string `json:"scsi_revision"`
	UserCapacity    struct {
		Bytes int64 `json:"bytes"`
	} `json:"user_capacity"`
	NvmeTotalCapacity int64 `json:"nvme_total_capacity"`
	RotationRate      *int  `json:"rotation_rate"`
	SmartStatus       *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	PowerOnTime *struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
}

func parseJSON(data []byte, info *Info) error {
	var r jsonReport
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	info.Model = firstNonEmpty(r.ModelName, r.ScsiModelName, strings.TrimSpace(r.ScsiVendor+" "+r.ScsiProduct))
	info.Serial = strings.TrimSpace(r.SerialNumber)
	info.Firmware = firstNonEmpty(r.FirmwareVersion, r.ScsiRevision)
	info.Capacity = r.UserCapacity.Bytes
	if info.Capacity == 0 {
		info.Capacity = r.NvmeTotalCapacity
	}
	switch {
	case strings.EqualFold(r.Device.Protocol, "NVMe") || strings.EqualFold(r.Device.Type, "nvme"):
		info.Kind = "NVMe"
	case r.RotationRate != nil && *r.RotationRate == 0:
		info.Kind = "SSD"
	case r.RotationRate != nil && *r.RotationRate > 0:
		info.Kind = "HDD"
	}
	if r.SmartStatus != nil {
		info.Health = "FAILED"
		if r.SmartStatus.Passed {
			info.Health = "PASSED"
		}
	}
	if r.PowerOnTime != nil {
		info.PowerOnHours = r.PowerOnTime.Hours
	}
	return nil
}

var (
	// A capacity in bytes ("4,000,787,030,016 bytes [4.00 TB]",
	// "1.000.204.886.016 [1,00 TB]") or with a decimal unit ("4000.7 GB").
	reBytes    = regexp.MustCompile(`^([\d][\d,.'\s\x{00a0}]*?)\s*(?:bytes|\[)`)
	reSized    = regexp.MustCompile(`(?i)^([\d]+(?:[.,]\d+)?)\s*([KMGTP])B`)
	reLeadNum  = regexp.MustCompile(`^\d[\d,.'\x{00a0}]*`)
	reRPM      = regexp.MustCompile(`(?i)(\d+)\s*rpm`)
	reAttrLine = regexp.MustCompile(`^\s*9\s+Power_On_Hours\S*\s`)
)

// capacityKeys lists the fields holding the disk size, best first.
var capacityKeys = []string{"user capacity", "total nvm capacity", "namespace 1 size/capacity", "disk size"}

func parseText(data []byte, info *Info) {
	fields := map[string]string{}
	var attrHours int64
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if reAttrLine.MatchString(line) {
			attrHours = attributeRaw(line)
			continue
		}
		if strings.Contains(strings.ToLower(line), "accumulated power on time") {
			// SCSI: "Accumulated power on time, hours:minutes 12345:30"
			if i := strings.LastIndexAny(line, " \t"); i >= 0 {
				attrHours = leadingInt(strings.TrimSpace(line[i:]))
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.Join(strings.Fields(k), " "))
		v = strings.TrimSpace(v)
		if _, seen := fields[k]; !seen && v != "" {
			fields[k] = v
		}
	}

	get := func(keys ...string) string {
		for _, k := range keys {
			if v := fields[k]; v != "" {
				return v
			}
		}
		return ""
	}

	info.Model = get("device model", "model number", "model")
	if info.Model == "" {
		// SCSI/SAS disks report vendor and product separately.
		info.Model = strings.TrimSpace(get("vendor") + " " + get("product"))
	}
	info.Serial = get("serial number")
	info.Firmware = get("firmware version", "firmware", "revision")
	for _, k := range capacityKeys {
		if c := parseCapacity(fields[k]); c > 0 {
			info.Capacity = c
			break
		}
	}

	iface := strings.ToLower(get("interface", "transport protocol"))
	rotation := strings.ToLower(get("rotation rate"))
	switch {
	case strings.Contains(iface, "nvm") || get("nvme version", "total nvm capacity", "pci vendor/subsystem id") != "":
		info.Kind = "NVMe"
	case strings.Contains(rotation, "solid state") || strings.Contains(rotation, "ssd"):
		info.Kind = "SSD"
	case reRPM.MatchString(rotation):
		info.Kind = "HDD"
	}

	// ATA disks report "FAILED!".
	info.Health = strings.TrimRight(get("smart overall-health self-assessment test result", "smart health status", "health status"), "!")
	if h := get("power on hours"); h != "" {
		info.PowerOnHours = leadingInt(h)
	} else {
		info.PowerOnHours = attrHours
	}
}

// attributeRaw returns the raw value of a row of smartctl's attribute table,
// in either the standard (-A) or brief (-x) format. The raw value follows the
// WHEN_FAILED column and may carry a suffix ("1234h+56m+07.123s", "1234 (12 0 0)").
func attributeRaw(line string) int64 {
	f := strings.Fields(line)
	for i := 3; i < len(f)-1; i++ {
		switch f[i] {
		case "-", "FAILING_NOW", "In_the_past", "NOW", "Past":
			return leadingInt(f[i+1])
		}
	}
	return 0
}

func parseCapacity(v string) int64 {
	if v == "" {
		return 0
	}
	if m := reBytes.FindStringSubmatch(v); m != nil {
		if n := leadingInt(m[1]); n > 0 {
			return n
		}
	}
	if m := reSized.FindStringSubmatch(v); m != nil {
		f, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		if err != nil {
			return 0
		}
		exp := strings.Index("KMGTP", strings.ToUpper(m[2])) + 1
		return int64(math.Round(f * math.Pow(1000, float64(exp))))
	}
	return 0
}

// leadingInt reads the number at the start of s, ignoring thousands separators.
func leadingInt(s string) int64 {
	m := reLeadNum.FindString(strings.TrimSpace(s))
	var digits strings.Builder
	for _, r := range m {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	n, _ := strconv.ParseInt(digits.String(), 10, 64)
	return n
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
