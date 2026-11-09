package fx

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestParseRate(t *testing.T) {
	tests := []struct {
		in      string
		want    Rate
		wantErr bool
	}{
		{"4", 40_000_000_000, false},
		{"4.4712", 44_712_000_000, false},
		{"0.2236", 2_236_000_000, false},
		{"0.0000000001", 1, false},
		{"007.5", 75_000_000_000, false},
		{"1.1234567890", 11_234_567_890, false},
		{"99999999.9999999999", 999_999_999_999_999_999, false},
		{"99999999", 999_999_990_000_000_000, false},
		{"", 0, true},
		{"-1", 0, true},
		{"+1", 0, true},
		{"0", 0, true},
		{"0.0", 0, true},
		{"0.00000000000", 0, true},
		{"1e3", 0, true},
		{"abc", 0, true},
		{"1.2.3", 0, true},
		{".5", 0, true},
		{"5.", 0, true},
		{" 5", 0, true},
		{"1.12345678901", 0, true},
		{"100000000", 0, true},
		{"100000000.0", 0, true},
		{"NaN", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRate(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRate(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRate) {
					t.Errorf("error %v does not wrap ErrInvalidRate", err)
				}
				return
			}
			if got != tt.want {
				t.Errorf("ParseRate(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestRateString(t *testing.T) {
	tests := []struct {
		in   Rate
		want string
	}{
		{44_712_000_000, "4.4712"},
		{40_000_000_000, "4"},
		{2_236_000_000, "0.2236"},
		{1, "0.0000000001"},
		{Rate(RateScale), "1"},
		{MaxRate, "99999999.9999999999"},
		{0, "0"},
		{-15_000_000_000, "-1.5"},
		{Rate(math.MinInt64), "-922337203.6854775808"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Rate(%d).String() = %q, want %q", int64(tt.in), got, tt.want)
		}
	}
}

func TestRateFromFloat(t *testing.T) {
	tests := []struct {
		in      float64
		want    Rate
		wantErr bool
	}{
		{4.4712, 44_712_000_000, false},
		{0.2236, 2_236_000_000, false},
		{1, Rate(RateScale), false},
		{1.00000000004, 10_000_000_000, false},
		{1.00000000006, 10_000_000_001, false},
		{math.NaN(), 0, true},
		{math.Inf(1), 0, true},
		{math.Inf(-1), 0, true},
		{0, 0, true},
		{-1, 0, true},
		{1e-12, 0, true},
		{1e9, 0, true},
	}
	for _, tt := range tests {
		got, err := RateFromFloat(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("RateFromFloat(%v) err = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("RateFromFloat(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestConvert(t *testing.T) {
	half := Rate(RateScale / 2)
	tests := []struct {
		name    string
		amount  int64
		rate    Rate
		want    int64
		wantErr bool
	}{
		{"identity", 12345, Rate(RateScale), 12345, false},
		{"usd to myr", 10_000, 44_712_000_000, 44_712, false},
		{"tie to even down", 1, half, 0, false},
		{"tie to even up", 3, half, 2, false},
		{"tie 5 halves", 5, half, 2, false},
		{"tie 7 halves", 7, half, 4, false},
		{"negative tie even", -3, half, -2, false},
		{"negative tie to zero", -1, half, 0, false},
		{"negative tie 5", -5, half, -2, false},
		{"round up above tie", 10, 5_000_000_001, 5, false},
		{"round up non tie", 7, 3_000_000_000, 2, false},
		{"round up non tie 2", 8, 3_000_000_000, 2, false},
		{"negative non tie", -7, 3_333_333_333, -2, false},
		{"zero amount", 0, 44_712_000_000, 0, false},
		{"huge exact", math.MaxInt64, Rate(RateScale), math.MaxInt64, false},
		{"huge min exact", math.MinInt64, Rate(RateScale), math.MinInt64, false},
		{"huge halves", math.MaxInt64 - 1, half, (math.MaxInt64 - 1) / 2, false},
		{"overflow", math.MaxInt64, 20_000_000_000, 0, true},
		{"overflow negative", math.MinInt64, 20_000_000_000, 0, true},
		{"big rate no overflow", 1 << 40, MaxRate / 1000, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert(tt.amount, tt.rate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrOverflow) {
					t.Errorf("error %v does not wrap ErrOverflow", err)
				}
				return
			}
			if tt.name == "big rate no overflow" {
				return // only checks that huge products do not overflow internally
			}
			if got != tt.want {
				t.Errorf("Convert(%d, %s) = %d, want %d", tt.amount, tt.rate, got, tt.want)
			}
		})
	}
}

func TestInvert(t *testing.T) {
	tests := []struct {
		name    string
		in      Rate
		want    Rate
		wantErr bool
	}{
		{"one", Rate(RateScale), Rate(RateScale), false},
		{"two", 20_000_000_000, 5_000_000_000, false},
		{"myr", 44_712_000_000, 2_236_536_053, false},
		{"tie 12.5 to even", 8_000_000_000_000_000_000, 12, false},
		{"tie 1562.5 to even", 64_000_000_000_000_000, 1562, false},
		{"largest rate", Rate(math.MaxInt64), 11, false},
		{"overflows", 1, 0, true},
		{"zero", 0, 0, true},
		{"negative", -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Invert(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Invert(%d) = %d, want %d", int64(tt.in), int64(got), int64(tt.want))
			}
		})
	}
}

func TestMul(t *testing.T) {
	tests := []struct {
		name    string
		a, b    Rate
		want    Rate
		wantErr bool
	}{
		{"identity", 44_712_000_000, Rate(RateScale), 44_712_000_000, false},
		{"pivot", 44_712_000_000, 2_236_536_053, 10_000_000_000, false},
		{"tie to even down", 5_000_000_000, 5, 2, false}, // 0.5 * 5e-10 = 2.5e-10 -> 2
		{"tie to even up", 5_000_000_000, 7, 4, false},   // 3.5 -> 4
		{"overflow", MaxRate, MaxRate, 0, true},
		{"rounds to zero", 1, 1, 0, true},
		{"zero a", 0, 1, 0, true},
		{"negative b", 1, -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Mul(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Mul(%s, %s) = %s, want %s", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestScanValueRoundTrip(t *testing.T) {
	rates := []Rate{1, 2_236_000_000, 44_712_000_000, Rate(RateScale), 40_000_000_000, MaxRate}
	for _, r := range rates {
		v, err := r.Value()
		if err != nil {
			t.Fatal(err)
		}
		s, ok := v.(string)
		if !ok || s != r.String() {
			t.Fatalf("Value() = %#v, want canonical string %q", v, r.String())
		}
		var back Rate
		if err := back.Scan(s); err != nil {
			t.Fatalf("Scan(string %q): %v", s, err)
		}
		if back != r {
			t.Errorf("string round trip %s -> %s", r, back)
		}
		var fromBytes Rate
		if err := fromBytes.Scan([]byte(s)); err != nil || fromBytes != r {
			t.Errorf("bytes round trip %s -> %s (%v)", r, fromBytes, err)
		}
	}
}

func TestScanTypes(t *testing.T) {
	tests := []struct {
		name    string
		src     any
		want    Rate
		wantErr bool
	}{
		{"numeric bytes", []byte("4.4712000000"), 44_712_000_000, false},
		{"float", 4.4712, 44_712_000_000, false},
		{"int whole", int64(4), 40_000_000_000, false},
		{"int zero", int64(0), 0, true},
		{"int negative", int64(-3), 0, true},
		{"int too large", int64(100_000_000), 0, true},
		{"bad string", "abc", 0, true},
		{"bad float", math.NaN(), 0, true},
		{"nil", nil, 0, true},
		{"unsupported", true, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rate(7)
			err := r.Scan(tt.src)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if r != 7 {
					t.Errorf("rate modified on error: %d", r)
				}
				return
			}
			if r != tt.want {
				t.Errorf("got %s, want %s", r, tt.want)
			}
		})
	}
}

func TestJSON(t *testing.T) {
	b, err := json.Marshal(struct {
		R Rate `json:"rate"`
	}{44_712_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"rate":"4.4712"}` {
		t.Errorf("marshal = %s", b)
	}

	var out struct {
		R Rate `json:"rate"`
	}
	if err := json.Unmarshal([]byte(`{"rate":"0.2236"}`), &out); err != nil || out.R != 2_236_000_000 {
		t.Errorf("unmarshal = %v, %v", out.R, err)
	}
	for _, bad := range []string{`{"rate":4.4712}`, `{"rate":"-1"}`, `{"rate":"x"}`} {
		if err := json.Unmarshal([]byte(bad), &out); err == nil {
			t.Errorf("unmarshal %s: expected error", bad)
		}
	}
}

func TestRateFromAmounts(t *testing.T) {
	cases := []struct {
		from, to int64
		want     string
		wantErr  bool
	}{
		{10000, 44712, "4.4712", false},
		{3, 10, "3.3333333333", false},
		{3, 20, "6.6666666667", false}, // 6.66666... rounds up
		{1, 1, "1", false},
		{0, 5, "", true},
		{5, 0, "", true},
		{-1, 5, "", true},
		{math.MaxInt64, 1, "", true}, // rounds to zero
	}
	for _, tc := range cases {
		got, err := RateFromAmounts(tc.from, tc.to)
		if tc.wantErr {
			if err == nil {
				t.Errorf("RateFromAmounts(%d, %d) = %s, want error", tc.from, tc.to, got)
			}
			continue
		}
		if err != nil || got.String() != tc.want {
			t.Errorf("RateFromAmounts(%d, %d) = %s, %v; want %s", tc.from, tc.to, got, err, tc.want)
		}
	}
}
