package smart

import (
	"errors"
	"testing"
)

const ataText = `smartctl 7.3 2022-02-28 r5338 [x86_64-linux-6.1.0] (local build)
Copyright (C) 2002-22, Bruce Allen, Christian Franke, www.smartmontools.org

=== START OF INFORMATION SECTION ===
Model Family:     Western Digital Red
Device Model:     WDC WD40EFRX-68N32N0
Serial Number:    WD-WCC7K1234567
LU WWN Device Id: 5 0014ee 2b9c8d7e6
Firmware Version: 82.00A82
User Capacity:    4.000.787.030.016 bytes [4,00 TB]
Sector Sizes:     512 bytes logical, 4096 bytes physical
Rotation Rate:    5400 rpm
Local Time is:    Sat Oct 10 12:00:00 2026 CEST
SMART support is: Enabled

=== START OF READ SMART DATA SECTION ===
SMART overall-health self-assessment test result: PASSED

ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
  1 Raw_Read_Error_Rate     0x002f   200   200   051    Pre-fail  Always       -       0
  9 Power_On_Hours          0x0032   061   061   000    Old_age   Always       -       28734
 12 Power_Cycle_Count       0x0032   100   100   000    Old_age   Always       -       112
`

const ataBrief = `Device Model:     ST2000DM008-2FR102
Serial Number:    ZFL0ABCD
Firmware Version: 0001
User Capacity:    2,000,398,934,016 bytes [2.00 TB]
Rotation Rate:    7200 rpm
SMART overall-health self-assessment test result: FAILED!
ID# ATTRIBUTE_NAME          FLAGS    VALUE WORST THRESH FAIL RAW_VALUE
  9 Power_On_Hours          -O--CK   076   076   000    -    21345h+12m+05.123s
`

const ssdText = `Device Model:     Samsung SSD 860 EVO 500GB
Serial Number:    S3Z1NB0K123456X
Firmware Version: RVT02B6Q
User Capacity:    500,107,862,016 bytes [500 GB]
Rotation Rate:    Solid State Device
SMART overall-health self-assessment test result: PASSED
  9 Power_On_Hours          0x0032   095   095   000    Old_age   Always       -       19876
`

const nvmeText = `=== START OF INFORMATION SECTION ===
Model Number:                       Samsung SSD 970 EVO Plus 1TB
Serial Number:                      S4EWNX0R123456A
Firmware Version:                   2B2QEXM7
PCI Vendor/Subsystem ID:            0x144d
Total NVM Capacity:                 1,000,204,886,016 [1.00 TB]
Namespace 1 Size/Capacity:          1,000,204,886,016 [1.00 TB]

=== START OF SMART DATA SECTION ===
SMART overall-health self-assessment test result: PASSED
Temperature:                        35 Celsius
Power On Hours:                     4,321
`

const scsiText = `Vendor:               SEAGATE
Product:              ST4000NM0023
Revision:             0004
User Capacity:        4,000,787,030,016 bytes [4.00 TB]
Rotation Rate:        7200 rpm
Serial number:        Z1Z2ABCD0000C1234567
Transport protocol:   SAS (SPL-3)
SMART Health Status: OK
Accumulated power on time, hours:minutes 41234:56
`

const crystalText = `----------------------------------------------------------------------------
 (01) WDC WD40EFRX-68N32N0
----------------------------------------------------------------------------
           Model : WDC WD40EFRX-68N32N0
        Firmware : 82.00A82
   Serial Number : WD-WCC7K7654321
       Disk Size : 4000.7 GB (8.4/137.4/4000.7/4000.7)
       Interface : Serial ATA
   Rotation Rate : 5400 RPM
  Power On Hours : 12345 hours
     Temperature : 30 C (86 F)
   Health Status : Good
`

const jsonATA = `{
  "json_format_version": [1, 0],
  "device": {"name": "/dev/sda", "type": "sat", "protocol": "ATA"},
  "model_family": "Western Digital Red",
  "model_name": "WDC WD40EFRX-68N32N0",
  "serial_number": "WD-WCC7K1234567",
  "firmware_version": "82.00A82",
  "user_capacity": {"blocks": 7814037168, "bytes": 4000787030016},
  "rotation_rate": 5400,
  "smart_status": {"passed": true},
  "power_on_time": {"hours": 28734}
}`

const jsonNVMe = `{
  "device": {"name": "/dev/nvme0", "type": "nvme", "protocol": "NVMe"},
  "model_name": "Samsung SSD 970 EVO Plus 1TB",
  "serial_number": "S4EWNX0R123456A",
  "firmware_version": "2B2QEXM7",
  "nvme_total_capacity": 1000204886016,
  "smart_status": {"passed": false},
  "power_on_time": {"hours": 4321}
}`

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Info
	}{
		{"ata", ataText, Info{"WDC WD40EFRX-68N32N0", "WD-WCC7K1234567", "82.00A82", 4000787030016, "HDD", "PASSED", 28734}},
		{"ata brief", ataBrief, Info{"ST2000DM008-2FR102", "ZFL0ABCD", "0001", 2000398934016, "HDD", "FAILED", 21345}},
		{"ssd", ssdText, Info{"Samsung SSD 860 EVO 500GB", "S3Z1NB0K123456X", "RVT02B6Q", 500107862016, "SSD", "PASSED", 19876}},
		{"nvme", nvmeText, Info{"Samsung SSD 970 EVO Plus 1TB", "S4EWNX0R123456A", "2B2QEXM7", 1000204886016, "NVMe", "PASSED", 4321}},
		{"scsi", scsiText, Info{"SEAGATE ST4000NM0023", "Z1Z2ABCD0000C1234567", "0004", 4000787030016, "HDD", "OK", 41234}},
		{"crystaldiskinfo", crystalText, Info{"WDC WD40EFRX-68N32N0", "WD-WCC7K7654321", "82.00A82", 4000700000000, "HDD", "Good", 12345}},
		{"json ata", jsonATA, Info{"WDC WD40EFRX-68N32N0", "WD-WCC7K1234567", "82.00A82", 4000787030016, "HDD", "PASSED", 28734}},
		{"json nvme", jsonNVMe, Info{"Samsung SSD 970 EVO Plus 1TB", "S4EWNX0R123456A", "2B2QEXM7", 1000204886016, "NVMe", "FAILED", 4321}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("\n got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestParseNoData(t *testing.T) {
	for _, in := range []string{"", "ciao", "foo: bar\n"} {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrNoData) {
			t.Errorf("Parse(%q) err = %v, want ErrNoData", in, err)
		}
	}
	if _, err := Parse([]byte("{not json")); err == nil {
		t.Error("invalid JSON accepted")
	}
}
